package forge_test

import (
	"encoding/hex"
	"net/http"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/forge"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/fake"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/gitea"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/github"
	"git.codigosh.com/CodigoSH/LastHelp/internal/forge/gitlab"
)

const secret = "a shared secret"

// TestEachForgeVerifiesItsOwnDeliveries.
func TestEachForgeVerifiesItsOwnDeliveries(t *testing.T) {
	gh, ghBody := fake.GitHubDelivery(secret, "closed", 7, "closed")
	gl, glBody := fake.GitLabDelivery(fake.GitLabSecret(secret), "close", 7, "closed", nil, nil)
	gt, gtBody := fake.GiteaDelivery(secret, "closed", 7, "closed")

	if err := (github.Receiver{}).Verify(gh, ghBody, secret); err != nil {
		t.Errorf("github refused its own delivery: %v", err)
	}
	if err := (gitlab.Receiver{}).Verify(gl, glBody, fake.GitLabSecret(secret)); err != nil {
		t.Errorf("gitlab refused its own delivery: %v", err)
	}
	if err := (gitea.Receiver{}).Verify(gt, gtBody, secret); err != nil {
		t.Errorf("gitea refused its own delivery: %v", err)
	}
}

// TestNoGenericVerifierWouldWork is the test this whole design exists for.
//
// GitHub and Gitea both sign the body, but with different headers. GitLab signs
// a composed message of id, timestamp and body, with a key that arrives
// base64-encoded behind a prefix. One verifier for all three would reject
// GitLab entirely, or — worse, if written to be lenient — accept whatever it
// could not check.
//
// Crossing the receivers over is how that gets noticed the day somebody
// "simplifies" the four into one.
func TestNoGenericVerifierWouldWork(t *testing.T) {
	gh, ghBody := fake.GitHubDelivery(secret, "closed", 7, "closed")
	gl, glBody := fake.GitLabDelivery(fake.GitLabSecret(secret), "close", 7, "closed", nil, nil)
	gt, gtBody := fake.GiteaDelivery(secret, "closed", 7, "closed")

	crossed := []struct {
		name     string
		receiver forge.Receiver
		headers  http.Header
		body     []byte
		secret   string
	}{
		{"github reading a gitlab delivery", github.Receiver{}, gl, glBody, secret},
		{"github reading a gitea delivery", github.Receiver{}, gt, gtBody, secret},
		{"gitlab reading a github delivery", gitlab.Receiver{}, gh, ghBody, fake.GitLabSecret(secret)},
		{"gitea reading a gitlab delivery", gitea.Receiver{}, gl, glBody, secret},
	}

	for _, c := range crossed {
		if err := c.receiver.Verify(c.headers, c.body, c.secret); err == nil {
			t.Errorf("%s: accepted it", c.name)
		}
	}
}

// TestATamperedBodyIsRefused. The signature is over the body; changing a byte
// has to break it, or the signature is decoration.
func TestATamperedBodyIsRefused(t *testing.T) {
	gh, body := fake.GitHubDelivery(secret, "closed", 7, "closed")
	tampered := append([]byte(nil), body...)
	tampered[len(tampered)-2] = '!'

	if err := (github.Receiver{}).Verify(gh, tampered, secret); err == nil {
		t.Error("a changed body kept its signature")
	}
}

func TestTheWrongSecretIsRefused(t *testing.T) {
	gh, body := fake.GitHubDelivery(secret, "closed", 7, "closed")
	if err := (github.Receiver{}).Verify(gh, body, "a different secret"); err == nil {
		t.Error("somebody else's secret verified")
	}
}

// TestAMissingSignatureIsRefused: absence is not permission.
func TestAMissingSignatureIsRefused(t *testing.T) {
	_, body := fake.GitHubDelivery(secret, "closed", 7, "closed")
	if err := (github.Receiver{}).Verify(http.Header{}, body, secret); err == nil {
		t.Error("a delivery with no signature at all was accepted")
	}
	if err := (gitea.Receiver{}).Verify(http.Header{}, body, secret); err == nil {
		t.Error("a delivery with no signature at all was accepted")
	}
	if err := (gitlab.Receiver{}).Verify(http.Header{}, body, secret); err == nil {
		t.Error("a delivery with no signature at all was accepted")
	}
}

// TestGitLabsLegacyTokenStillWorks — instances configured years ago still send
// it, and refusing them would break upgrades rather than protect anybody.
func TestGitLabsLegacyToken(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Gitlab-Token", secret)
	if err := (gitlab.Receiver{}).Verify(headers, []byte(`{}`), secret); err != nil {
		t.Errorf("the legacy token was refused: %v", err)
	}
	headers.Set("X-Gitlab-Token", "wrong")
	if err := (gitlab.Receiver{}).Verify(headers, []byte(`{}`), secret); err == nil {
		t.Error("the wrong legacy token was accepted")
	}
}

// TestTheThreeVocabulariesMeanTheSameThreeThings.
//
// "closed" on GitHub, "close" on GitLab, "closed" on Gitea. And labelling:
// "labeled" on GitHub, "label_updated" on Gitea, and on GitLab no action at all
// — it arrives as "update" and has to be found by diffing. Anybody mapping
// actions one-to-one loses every GitLab label change, silently.
func TestTheThreeVocabulariesMeanTheSameThreeThings(t *testing.T) {
	closed := []struct {
		name     string
		receiver forge.Receiver
		headers  http.Header
		body     []byte
	}{}

	gh, ghBody := fake.GitHubDelivery(secret, "closed", 7, "closed")
	gl, glBody := fake.GitLabDelivery(fake.GitLabSecret(secret), "close", 7, "closed", nil, nil)
	gt, gtBody := fake.GiteaDelivery(secret, "closed", 7, "closed")
	closed = append(closed,
		struct {
			name     string
			receiver forge.Receiver
			headers  http.Header
			body     []byte
		}{"github", github.Receiver{}, gh, ghBody},
		struct {
			name     string
			receiver forge.Receiver
			headers  http.Header
			body     []byte
		}{"gitlab", gitlab.Receiver{}, gl, glBody},
		struct {
			name     string
			receiver forge.Receiver
			headers  http.Header
			body     []byte
		}{"gitea", gitea.Receiver{}, gt, gtBody},
	)

	for _, c := range closed {
		event, err := c.receiver.Parse(c.headers, c.body)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if event.Kind != forge.EventClosed {
			t.Errorf("%s: an issue closing came through as %q", c.name, event.Kind)
		}
		if event.Number != 7 {
			t.Errorf("%s: issue number came through as %d", c.name, event.Number)
		}
		if event.State != forge.Closed {
			t.Errorf("%s: state came through as %q", c.name, event.State)
		}
		if event.DeliveryID == "" {
			t.Errorf("%s: no delivery id, so a replay could not be recognised", c.name)
		}
	}
}

// TestGitLabLabelChangesAreFoundByDiffing is the one an action table misses.
func TestGitLabLabelChangesAreFoundByDiffing(t *testing.T) {
	// A relabel: it arrives as "update", and only the diff says what happened.
	headers, body := fake.GitLabDelivery(fake.GitLabSecret(secret), "update", 7, "opened",
		[]string{"bug", "confirmed"}, []string{"bug"})

	event, err := (gitlab.Receiver{}).Parse(headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != forge.EventLabelled {
		t.Errorf("a GitLab relabel came through as %q; it has no labelling action, "+
			"so an action table loses every one of them", event.Kind)
	}

	// And an update that changed something else is not a relabel.
	headers, body = fake.GitLabDelivery(fake.GitLabSecret(secret), "update", 7, "opened",
		[]string{"bug"}, []string{"bug"})
	event, err = (gitlab.Receiver{}).Parse(headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind == forge.EventLabelled {
		t.Error("an update with unchanged labels was read as a relabel")
	}
}

func TestGiteaLabellingHasItsOwnWords(t *testing.T) {
	for _, action := range []string{"label_updated", "label_cleared"} {
		headers, body := fake.GiteaDelivery(secret, action, 7, "open")
		event, err := (gitea.Receiver{}).Parse(headers, body)
		if err != nil {
			t.Fatal(err)
		}
		if event.Kind != forge.EventLabelled {
			t.Errorf("%q came through as %q", action, event.Kind)
		}
	}
}

// TestSomethingThatIsNotAnIssueEventIsIgnored.
//
// A forge sends deliveries for a great many things. Treating a push as an issue
// event is how an unrelated commit closes somebody's report.
func TestSomethingThatIsNotAnIssueEventIsIgnored(t *testing.T) {
	headers, body := fake.GitHubDelivery(secret, "closed", 7, "closed")
	headers.Set("X-GitHub-Event", "push")

	event, err := (github.Receiver{}).Parse(headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if event.Kind != forge.EventIgnored {
		t.Errorf("a push came through as %q", event.Kind)
	}
}

// TestAKnownSignatureVector pins the algorithm against a value computed by
// something that is not this code.
//
// Every other test here signs with the fake and verifies with the adapter, so
// both halves could drift together and still agree — a round trip proves the
// two sides match, not that either is right. This value came out of Python's
// hmac, and the first version of this test used one written from memory, which
// was wrong: the fixture is only worth anything when it is computed rather than
// remembered.
func TestAKnownSignatureVector(t *testing.T) {
	// HMAC-SHA256 of the exact bytes `{"hello":"world"}` with key `it's a secret`.
	const (
		body = `{"hello":"world"}`
		key  = "it's a secret"
		want = "8d4063f0a81aa1531d9891a028a68cf2bb537ecdf0e82557674d71e168d570f9"
	)

	headers := http.Header{}
	headers.Set("X-Hub-Signature-256", "sha256="+want)
	if err := (github.Receiver{}).Verify(headers, []byte(body), key); err != nil {
		t.Fatalf("the known GitHub vector failed: %v", err)
	}

	headers = http.Header{}
	headers.Set("X-Forgejo-Signature", want)
	if err := (gitea.Receiver{}).Verify(headers, []byte(body), key); err != nil {
		t.Fatalf("the known Forgejo vector failed: %v", err)
	}

	if _, err := hex.DecodeString(want); err != nil {
		t.Fatal("the vector itself is not hex")
	}

	// And GitLab's, which signs a composed message rather than the body — the
	// difference a single generic verifier would paper over.
	glHeaders := http.Header{}
	glHeaders.Set("webhook-id", "delivery-gitlab-1")
	glHeaders.Set("webhook-timestamp", "1700000000")
	glHeaders.Set("webhook-signature", "v1,ZvKOs2zQbG3rXvhco7SjBL3zXKUiQ7x2t0nHYfxJDCM=")
	if err := (gitlab.Receiver{}).Verify(glHeaders, []byte(`{}`), fake.GitLabSecret("a shared secret")); err != nil {
		t.Fatalf("the known GitLab vector failed: %v", err)
	}
}
