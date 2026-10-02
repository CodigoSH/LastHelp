package web

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

func aPhoto(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 5), 0x40, uint8(y * 7), 0xff})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// picture sends a picture the way the page does, with the follow link when
// there is one.
func (i *instance) picture(method, path string, body []byte, followToken string) *http.Response {
	i.t.Helper()
	req, err := http.NewRequest(method, i.server.URL+path, bytes.NewReader(body))
	if err != nil {
		i.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if token := i.csrf(); token != "" {
		req.Header.Set("X-CSRF-Token", token)
	}
	if followToken != "" {
		req.Header.Set(FollowTokenHeader, followToken)
	}
	resp, err := i.client.Do(req)
	if err != nil {
		i.t.Fatal(err)
	}
	return resp
}

func pictureSetup(t *testing.T) (team, public *instance, id, token string) {
	team = newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	public = team.anonymous()
	id, token = public.submit("codigosh", "lastboard", "bug", "crash on import")
	return team, public, id, token
}

// TestAPictureWaitsWithItsReport: the reporter's picture is seen by the
// reporter and the team while the report waits, by nobody else, and by
// everybody once it is published.
func TestAPictureWaitsWithItsReport(t *testing.T) {
	team, public, id, token := pictureSetup(t)
	stranger := team.anonymous()

	resp := public.picture(http.MethodPost, "/api/items/"+id+"/images", aPhoto(t), token)
	added := decode(t, resp)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("the reporter's picture was refused: %d %v", resp.StatusCode, added)
	}
	url, _ := added["url"].(string)
	thumb, _ := added["thumb"].(string)

	code := func(who *instance, path string) int { return status(t, who.do(http.MethodGet, path, nil)) }
	if got := code(stranger, url); got != http.StatusNotFound {
		t.Errorf("a stranger saw a waiting picture: %d", got)
	}
	if got := code(public, url+"?k="+token); got != http.StatusOK {
		t.Errorf("the reporter could not see their own picture: %d", got)
	}
	if got := code(team, thumb); got != http.StatusOK {
		t.Errorf("the team could not see the picture it has to judge: %d", got)
	}
	if pics, _ := decode(t, stranger.do(http.MethodGet, "/api/items/"+id, nil))["images"].([]any); len(pics) != 0 {
		t.Errorf("the report lists %d pictures to a stranger before it is published", len(pics))
	}

	_ = status(t, team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"}))
	served := stranger.do(http.MethodGet, url, nil)
	_ = served.Body.Close()
	if served.StatusCode != http.StatusOK {
		t.Fatalf("a published picture answered %d", served.StatusCode)
	}
	for header, want := range map[string]string{
		"Content-Type":            "image/jpeg",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
		"Cache-Control":           "private, no-cache",
	} {
		if got := served.Header.Get(header); got != want {
			t.Errorf("%s is %q, want %q", header, got, want)
		}
	}
	if pics, _ := decode(t, stranger.do(http.MethodGet, "/api/items/"+id, nil))["images"].([]any); len(pics) != 1 {
		t.Errorf("the published report lists %d pictures", len(pics))
	}
}

// TestOnlyTheReporterAndTheTeamAddPictures: a visitor adds nothing, the
// reporter adds to the report and their own follow-ups, the team to its
// replies.
func TestOnlyTheReporterAndTheTeamAddPictures(t *testing.T) {
	team, public, id, token := pictureSetup(t)
	_ = status(t, team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"}))
	visitor := team.anonymous()
	post := func(who *instance, comment, followToken string) int {
		path := "/api/items/" + id + "/images"
		if comment != "" {
			path += "?comment=" + comment
		}
		return status(t, who.picture(http.MethodPost, path, aPhoto(t), followToken))
	}

	if got := post(visitor, "", ""); got != http.StatusForbidden {
		t.Errorf("a visitor added a picture to the report: %d", got)
	}
	if got := post(team, "", ""); got != http.StatusForbidden {
		t.Errorf("the team added a picture to the reporter's report: %d", got)
	}

	idOf := func(resp *http.Response) string {
		body := decode(t, resp)
		n, _ := body["id"].(float64)
		return itoa(int64(n))
	}
	theirs := idOf(visitor.do(http.MethodPost, "/api/items/"+id+"/comments", map[string]string{"body": "me too"}))
	mine := idOf(public.do(http.MethodPost, "/api/items/"+id+"/comments", map[string]string{"body": "here it is", "token": token}))
	reply := idOf(team.do(http.MethodPost, "/api/items/"+id+"/reply", map[string]string{"body": "which version?"}))

	if got := post(public, theirs, token); got != http.StatusForbidden {
		t.Errorf("the reporter added a picture to a visitor's comment: %d", got)
	}
	if got := post(public, mine, token); got != http.StatusCreated {
		t.Errorf("the reporter could not add to their own follow-up: %d", got)
	}
	if got := post(team, reply, ""); got != http.StatusCreated {
		t.Errorf("the team could not add to its own reply: %d", got)
	}
	if got := post(public, reply, token); got != http.StatusForbidden {
		t.Errorf("the reporter added a picture to the team's reply: %d", got)
	}
}

func TestTheReporterRemovesTheirsAndTheTeamRemovesAny(t *testing.T) {
	team, public, id, token := pictureSetup(t)
	idOf := func(resp *http.Response) string {
		body := decode(t, resp)
		s, _ := body["id"].(string)
		return s
	}
	first := idOf(public.picture(http.MethodPost, "/api/items/"+id+"/images", aPhoto(t), token))
	second := idOf(public.picture(http.MethodPost, "/api/items/"+id+"/images", aPhoto(t), token))
	replyBody := decode(t, team.do(http.MethodPost, "/api/items/"+id+"/reply", map[string]string{"body": "a screenshot of ours"}))
	reply := itoa(int64(replyBody["id"].(float64)))
	ours := idOf(team.picture(http.MethodPost, "/api/items/"+id+"/images?comment="+reply, aPhoto(t), ""))

	remove := func(who *instance, image, followToken string) int {
		return status(t, who.picture(http.MethodDelete, "/api/items/"+id+"/images/"+image, nil, followToken))
	}
	if got := remove(team.anonymous(), first, ""); got != http.StatusForbidden {
		t.Errorf("a stranger removed a picture: %d", got)
	}
	if got := remove(public, ours, token); got != http.StatusForbidden {
		t.Errorf("the reporter removed the team's picture: %d", got)
	}
	if got := remove(public, first, token); got != http.StatusNoContent {
		t.Errorf("the reporter could not take back their own: %d", got)
	}
	if got := remove(team, second, ""); got != http.StatusNoContent {
		t.Errorf("the team could not take one out: %d", got)
	}
	if got := status(t, team.do(http.MethodGet, "/api/items/"+id+"/images/"+first, nil)); got != http.StatusNotFound {
		t.Errorf("a removed picture is still served: %d", got)
	}
}

func TestATeamCanTurnPicturesOffForItsBoard(t *testing.T) {
	team, public, id, token := pictureSetup(t)
	board := func() any {
		return decode(t, public.do(http.MethodGet, "/api/orgs/codigosh/repos/lastboard", nil))["accepts_images"]
	}
	if board() != true {
		t.Fatalf("a new board does not take pictures: %v", board())
	}
	edit := map[string]any{"name": "Lastboard", "forge_repo": "", "accepts_bugs": true, "accepts_ideas": true}
	edit["accepts_images"] = false
	if got := status(t, team.do(http.MethodPatch, "/api/orgs/codigosh/repos/lastboard", edit)); got != http.StatusOK {
		t.Fatalf("turning pictures off answered %d", got)
	}
	resp := public.picture(http.MethodPost, "/api/items/"+id+"/images", aPhoto(t), token)
	body := decode(t, resp)
	if resp.StatusCode != http.StatusConflict || body["error"] != "error.images_off" {
		t.Errorf("a picture on a board with them off answered %d %v", resp.StatusCode, body)
	}

	delete(edit, "accepts_images")
	_ = status(t, team.do(http.MethodPatch, "/api/orgs/codigosh/repos/lastboard", edit))
	if board() != false {
		t.Error("an edit that did not mention pictures turned them back on")
	}
}

func TestAnOversizedOrFalsePictureIsRefusedWithAReason(t *testing.T) {
	_, public, id, token := pictureSetup(t)
	resp := public.picture(http.MethodPost, "/api/items/"+id+"/images", make([]byte, item.MaxImageUpload+1), token)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("an oversized body answered %d", resp.StatusCode)
	}
	resp = public.picture(http.MethodPost, "/api/items/"+id+"/images", []byte("\xFF\xD8\xFF not a jpeg"), token)
	body := decode(t, resp)
	if resp.StatusCode != http.StatusBadRequest || body["error"] != "error.not_an_image" {
		t.Errorf("a false JPEG answered %d %v", resp.StatusCode, body)
	}
}
