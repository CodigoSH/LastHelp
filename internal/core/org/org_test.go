package org

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var epoch = time.Unix(1_700_000_000, 0)

func store(t *testing.T) Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return Store{DB: db}
}

func TestCreateAndFindAnOrganization(t *testing.T) {
	s := store(t)

	created, err := s.CreateOrganization(Organization{
		Slug: " CodigoSH ", Name: "CodigoSH", Forge: ForgeForgejo,
		BaseURL: "https://git.example.com", ForgeOwner: "CodigoSH",
	}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if created.Slug != "codigosh" {
		t.Errorf("slug stored as %q; addresses are normalised", created.Slug)
	}

	found, err := s.OrganizationBySlug("CODIGOSH")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != created.ID || found.Forge != ForgeForgejo {
		t.Errorf("looked up %+v", found)
	}
}

// TestAnOrganizationWithNoForgeIsValid.
//
// "none" is a supported way to run, not a placeholder. Everything except
// turning an item into an issue works without a forge, and the screens say
// there is none rather than offering buttons that fail.
func TestAnOrganizationWithNoForgeIsValid(t *testing.T) {
	s := store(t)
	created, err := s.CreateOrganization(Organization{Slug: "solo"}, epoch)
	if err != nil {
		t.Fatalf("an organisation with no forge was refused: %v", err)
	}
	if created.Forge != ForgeNone {
		t.Errorf("forge defaulted to %q, want none", created.Forge)
	}
}

func TestAnUnknownForgeIsRefused(t *testing.T) {
	s := store(t)
	_, err := s.CreateOrganization(Organization{Slug: "x", Forge: Forge("bitbucket")}, epoch)
	if !errors.Is(err, ErrUnknownForge) {
		t.Errorf("got %v, want ErrUnknownForge — a forge we cannot speak to must be refused "+
			"at the door rather than discovered when the first issue fails to be created", err)
	}
}

// TestBadAddressesAreRefused.
//
// A slug ends up in URLs, in chat and in documentation, and it is the one thing
// that cannot be changed later without breaking every link to it.
func TestBadAddressesAreRefused(t *testing.T) {
	s := store(t)
	for _, bad := range []string{
		"", " ", "-leading", "trailing-", "double--dash", "with space",
		"with_underscore", "with.dot", "with/slash", "../escape",
		strings.Repeat("a", maxSlugLength+1),
	} {
		if _, err := s.CreateOrganization(Organization{Slug: bad}, epoch); !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("slug %q was accepted (or failed with %v)", bad, err)
		}
	}
}

func TestTheSameAddressCannotBeUsedTwice(t *testing.T) {
	s := store(t)
	if _, err := s.CreateOrganization(Organization{Slug: "taken"}, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrganization(Organization{Slug: "TAKEN"}, epoch); !errors.Is(err, ErrSlugTaken) {
		t.Error("the same address was taken twice; normalisation is what makes this collide")
	}
}

func TestRepositoriesLiveUnderAnOrganization(t *testing.T) {
	s := store(t)
	o, err := s.CreateOrganization(Organization{Slug: "codigosh", Forge: ForgeForgejo}, epoch)
	if err != nil {
		t.Fatal(err)
	}

	r, err := s.CreateRepository(Repository{
		OrgID: o.ID, Slug: "lastboard", Name: "Lastboard",
		ForgeRepo: "CodigoSH/Lastboard", AcceptsBugs: true, AcceptsIdeas: true,
	}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if r.ModerationMode != ModerateAll {
		t.Errorf("moderation defaulted to %q; holding everything is the right default on day one", r.ModerationMode)
	}

	found, err := s.RepositoryBySlugs("codigosh", "lastboard")
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != r.ID || found.OrgID != o.ID {
		t.Errorf("resolving the public address gave %+v", found)
	}
}

// TestTheSameRepositoryNameCanExistInTwoOrganizations.
//
// Two organisations both having a "docs" is ordinary, and the uniqueness is per
// organisation for exactly that reason.
func TestTheSameRepositoryNameCanExistInTwoOrganizations(t *testing.T) {
	s := store(t)
	first, err := s.CreateOrganization(Organization{Slug: "one"}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateOrganization(Organization{Slug: "two"}, epoch)
	if err != nil {
		t.Fatal(err)
	}

	for _, orgID := range []int64{first.ID, second.ID} {
		if _, err := s.CreateRepository(Repository{
			OrgID: orgID, Slug: "docs", AcceptsBugs: true,
		}, epoch); err != nil {
			t.Fatalf("creating docs under organisation %d: %v", orgID, err)
		}
	}

	// And they are genuinely different rows reachable at different addresses.
	a, err := s.RepositoryBySlugs("one", "docs")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.RepositoryBySlugs("two", "docs")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Error("both addresses resolved to the same repository")
	}
}

func TestARepositoryUnderNoOrganizationIsRefused(t *testing.T) {
	s := store(t)
	_, err := s.CreateRepository(Repository{OrgID: 404, Slug: "ghost", AcceptsBugs: true}, epoch)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// TestARepositoryMustAcceptSomething.
//
// One that accepts neither bugs nor ideas is a public page with no way to use
// it, which reads as broken rather than as closed.
func TestARepositoryMustAcceptSomething(t *testing.T) {
	s := store(t)
	o, err := s.CreateOrganization(Organization{Slug: "codigosh"}, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepository(Repository{OrgID: o.ID, Slug: "empty"}, epoch); err == nil {
		t.Error("a repository accepting nothing was created")
	}
}

func TestMissingThingsSayTheyAreMissing(t *testing.T) {
	s := store(t)
	if _, err := s.OrganizationBySlug("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
	if _, err := s.RepositoryBySlugs("nope", "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestListsComeBackInAStableOrder(t *testing.T) {
	s := store(t)
	for _, name := range []string{"Zulu", "alpha", "Mike"} {
		if _, err := s.CreateOrganization(Organization{Slug: strings.ToLower(name), Name: name}, epoch); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListOrganizations()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "Mike", "Zulu"}
	for i, o := range list {
		if o.Name != want[i] {
			t.Fatalf("order is %v; a list that reshuffles between two loads is a list nobody trusts",
				names(list))
		}
	}
}

func names(list []Organization) []string {
	out := make([]string, 0, len(list))
	for _, o := range list {
		out = append(out, o.Name)
	}
	return out
}
