package web_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/xid"

	"github.com/kevinmchugh/mars-sim/internal/creaturelab"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/auth"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/db"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server/export"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server/species"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/server/sprites"
	"github.com/kevinmchugh/mars-sim/internal/creaturelab/web"
)

// The end-to-end test needs a Postgres it may wipe: point
// CREATURE_LAB_TEST_DATABASE_URL at a scratch database. Without it the test
// skips, so `go test ./...` passes on a machine with no Postgres.
const dbEnv = "CREATURE_LAB_TEST_DATABASE_URL"

const sprite = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><circle cx="64" cy="64" r="52" fill="#9c6" stroke="#1d1512" stroke-width="6"/></svg>`

type lab struct {
	t      *testing.T
	url    string
	key    string
	keyID  string
	q      *db.Queries
	client *http.Client // never follows redirects
}

func newLab(t *testing.T) *lab {
	dsn := os.Getenv(dbEnv)
	if dsn == "" {
		t.Skip(dbEnv + " not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrate(t, pool)

	ts := httptest.NewServer(nil)
	t.Cleanup(ts.Close)
	store := creaturelab.NewStore(pool)
	ts.Config.Handler = web.New(store, ts.URL).Handler()

	raw, err := auth.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.CreateAPIKey(ctx, db.CreateAPIKeyParams{ID: xid.New().String(), Name: "test", KeyHash: auth.Hash(raw)})
	if err != nil {
		t.Fatal(err)
	}
	return &lab{t: t, url: ts.URL, key: raw, keyID: key.ID, q: store.Queries, client: &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// migrate rebuilds the schema from the dbmate files' up sections.
func migrate(t *testing.T, pool *pgxpool.Pool) {
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("../../../creature-lab/db/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(b), "-- migrate:down")
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
}

// do sends a request with a bearer token (if any) and decodes a JSON reply
// into out (if non-nil), failing unless the status is want.
func (l *lab) do(method, path, token string, body any, want int, out any) *http.Response {
	l.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, l.url+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := l.client.Do(req)
	if err != nil {
		l.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		l.t.Fatalf("%s %s: status %d, want %d: %s", method, path, resp.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			l.t.Fatalf("%s %s: decode %s: %v", method, path, data, err)
		}
	}
	return resp
}

// multiFormSeed finds a seed whose species has a life of several forms.
func multiFormSeed(t *testing.T) int64 {
	for seed := int64(1); seed < 1000; seed++ {
		if len(creaturelab.Forms(creaturelab.Roll(seed))) >= 2 {
			return seed
		}
	}
	t.Fatal("no multi-form species in 1000 seeds")
	return 0
}

func TestAPIFlow(t *testing.T) {
	l := newLab(t)

	resp := l.do("GET", "/api/export", "", nil, http.StatusUnauthorized, nil)
	if !strings.Contains(resp.Header.Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource") {
		t.Fatalf("401 does not point at OAuth discovery: %q", resp.Header.Get("WWW-Authenticate"))
	}
	l.do("GET", "/api/export", "cl_not-a-key", nil, http.StatusUnauthorized, nil)
	l.do("POST", "/api/species", "", map[string]any{"seed": 1}, http.StatusUnauthorized, nil)

	seed := multiFormSeed(t)
	var sp species.Detail
	l.do("POST", "/api/species", l.key, map[string]any{"seed": seed}, http.StatusCreated, &sp)
	want := creaturelab.Roll(seed)
	if sp.Seed != seed || sp.Traits.Plural != want.Plural || len(sp.Forms) != len(creaturelab.Forms(want)) || sp.Complete {
		t.Fatalf("created %+v", sp)
	}

	var brief struct{ Brief string }
	l.do("GET", "/api/species/"+sp.ID+"/forms/1/brief", l.key, nil, http.StatusOK, &brief)
	if !strings.Contains(brief.Brief, want.Description()) {
		t.Fatal("brief lacks the field notes")
	}
	l.do("GET", "/api/species/"+sp.ID+"/forms/99/brief", l.key, nil, http.StatusUnprocessableEntity, nil)

	path := "/api/species/" + sp.ID + "/forms/0/candidates"
	l.do("POST", path, l.key, map[string]any{"svg": `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`}, http.StatusUnprocessableEntity, nil)
	var first, second sprites.Candidate
	l.do("POST", path, l.key, map[string]any{"svg": sprite, "note": "v1", "accept": true}, http.StatusCreated, &first)
	l.do("POST", path, l.key, map[string]any{"svg": sprite, "note": "v2"}, http.StatusCreated, &second)
	if !first.Accepted || second.Accepted {
		t.Fatalf("accepted flags: first %v, second %v", first.Accepted, second.Accepted)
	}
	l.do("POST", "/api/candidates/"+second.ID+"/accept", l.key, nil, http.StatusOK, nil)
	var cands struct{ Candidates []sprites.Candidate }
	l.do("GET", "/api/species/"+sp.ID+"/candidates?form=0", l.key, nil, http.StatusOK, &cands)
	accepted := 0
	for _, c := range cands.Candidates {
		if c.Accepted {
			accepted++
			if c.ID != second.ID {
				t.Fatal("accepting the second candidate left the first accepted")
			}
		}
	}
	if accepted != 1 || len(cands.Candidates) != 2 {
		t.Fatalf("%d candidates, %d accepted", len(cands.Candidates), accepted)
	}

	resp = l.do("GET", "/sprites/"+second.ID+".svg", l.key, nil, http.StatusOK, nil)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Fatalf("sprite content type %q", ct)
	}
	if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatal("sprite served without a sandbox CSP")
	}

	var exp export.Export
	l.do("GET", "/api/export", l.key, nil, http.StatusOK, &exp)
	if len(exp.Species) != 0 {
		t.Fatal("export lists an incomplete species")
	}
	for i := 1; i < len(sp.Forms); i++ {
		l.do("POST", "/api/species/"+sp.ID+"/forms/"+itoa(i)+"/candidates", l.key, map[string]any{"svg": sprite, "accept": true}, http.StatusCreated, nil)
	}
	l.do("GET", "/api/export", l.key, nil, http.StatusOK, &exp)
	if len(exp.Species) != 1 || !exp.Species[0].Complete || len(exp.Species[0].Sprites) != len(sp.Forms) {
		t.Fatalf("export of a complete species: %+v", exp)
	}

	// Anyone may list and view species and every candidate, accepted or
	// not, and open its sprite; nothing else is public.
	var public struct{ Species []species.Summary }
	l.do("GET", "/api/species", "", nil, http.StatusOK, &public)
	if len(public.Species) != 1 || public.Species[0].ID != sp.ID || !public.Species[0].Complete {
		t.Fatalf("public list: %+v", public)
	}
	var viewed species.Detail
	l.do("GET", "/api/species/"+sp.ID, "", nil, http.StatusOK, &viewed)
	if viewed.ID != sp.ID || len(viewed.Forms) != len(sp.Forms) || !viewed.Complete {
		t.Fatalf("public view: %+v", viewed)
	}
	l.do("GET", "/api/species/no-such-species", "", nil, http.StatusNotFound, nil)
	var anon struct{ Candidates []sprites.Candidate }
	l.do("GET", "/api/species/"+sp.ID+"/candidates?svg=true", "", nil, http.StatusOK, &anon)
	if len(anon.Candidates) != len(sp.Forms)+1 || anon.Candidates[0].SVG == "" {
		t.Fatalf("public candidates: %d, want %d with SVG", len(anon.Candidates), len(sp.Forms)+1)
	}
	var unaccepted sprites.Candidate
	l.do("GET", "/api/candidates/"+first.ID, "", nil, http.StatusOK, &unaccepted)
	if unaccepted.Accepted || unaccepted.SVG == "" {
		t.Fatalf("public unaccepted candidate: %+v", unaccepted)
	}
	l.do("GET", "/sprites/"+first.ID+".svg", "", nil, http.StatusOK, nil)
	l.do("GET", "/api/species/"+sp.ID+"/forms/0/brief", "", nil, http.StatusUnauthorized, nil)
	l.do("POST", "/api/candidates/"+first.ID+"/accept", "", nil, http.StatusUnauthorized, nil)
	l.do("PATCH", "/api/species/"+sp.ID, "", map[string]any{"notes": "x"}, http.StatusUnauthorized, nil)
	l.do("DELETE", "/api/species/"+sp.ID, "", nil, http.StatusUnauthorized, nil)

	l.do("DELETE", "/api/candidates/"+second.ID, l.key, nil, http.StatusOK, nil)
	l.do("GET", "/api/species/"+sp.ID, l.key, nil, http.StatusOK, &sp)
	if sp.Complete || sp.Forms[0].AcceptedID != "" {
		t.Fatal("deleting the accepted candidate left its slot filled")
	}
	l.do("DELETE", "/api/species/"+sp.ID, l.key, nil, http.StatusOK, nil)
	l.do("GET", "/api/species/"+sp.ID, l.key, nil, http.StatusNotFound, nil)
}

// The flow a claude.ai connector runs: register, authorize with a pasted
// key, trade the code for tokens, use and refresh them.
func TestOAuthFlow(t *testing.T) {
	l := newLab(t)
	const redirect = "https://claude.example/callback"

	var reg struct {
		ClientID string `json:"client_id"`
	}
	l.do("POST", "/oauth/register", "", map[string]any{"redirect_uris": []string{redirect}, "client_name": "test"}, http.StatusCreated, &reg)

	verifier := "a-verifier-that-is-long-enough-for-pkce-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	form := url.Values{
		"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {redirect},
		"state": {"xyz"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	resp, err := l.client.Get(l.url + "/oauth/authorize?" + form.Encode())
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("authorize page: %v %v", resp.StatusCode, err)
	}
	form.Set("api_key", "cl_wrong")
	if resp, _ = l.client.PostForm(l.url+"/oauth/authorize", form); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a wrong key should not authorize: %d", resp.StatusCode)
	}
	form.Set("api_key", l.key)
	resp, err = l.client.PostForm(l.url+"/oauth/authorize", form)
	if err != nil || resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("authorize: %v %v", resp.StatusCode, err)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Query().Get("state") != "xyz" || loc.Query().Get("code") == "" {
		t.Fatalf("redirect %s", loc)
	}

	token := func(v url.Values, want int) map[string]any {
		t.Helper()
		resp, err := l.client.PostForm(l.url+"/oauth/token", v)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != want {
			t.Fatalf("token: %d %v, want %d", resp.StatusCode, out, want)
		}
		return out
	}
	codeGrant := url.Values{"grant_type": {"authorization_code"}, "code": {loc.Query().Get("code")},
		"code_verifier": {verifier}, "client_id": {reg.ClientID}, "redirect_uri": {redirect}}
	tok := token(codeGrant, http.StatusOK)
	token(codeGrant, http.StatusBadRequest) // a code works once

	access, refresh := tok["access_token"].(string), tok["refresh_token"].(string)
	l.do("GET", "/api/export", access, nil, http.StatusOK, nil)

	refreshGrant := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}
	next := token(refreshGrant, http.StatusOK)
	token(refreshGrant, http.StatusBadRequest) // a refresh token works once
	l.do("GET", "/api/export", access, nil, http.StatusUnauthorized, nil)
	l.do("GET", "/api/export", next["access_token"].(string), nil, http.StatusOK, nil)

	// Deleting the key cuts off every token it approved.
	if err := l.q.DeleteAPIKey(context.Background(), l.keyID); err != nil {
		t.Fatal(err)
	}
	l.do("GET", "/api/export", next["access_token"].(string), nil, http.StatusUnauthorized, nil)
}

// A newcomer has no api key: they connect a client with a single-use invite,
// which mints their key.
func TestInviteFlow(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	const redirect = "https://claude.example/callback"
	var reg struct {
		ClientID string `json:"client_id"`
	}
	l.do("POST", "/oauth/register", "", map[string]any{"redirect_uris": []string{redirect}}, http.StatusCreated, &reg)

	invite := func(note string, expires time.Duration) string {
		t.Helper()
		raw, _ := auth.RandToken(auth.InvitePrefix)
		arg := db.CreateInviteParams{ID: xid.New().String(), CodeHash: auth.Hash(raw), Note: note}
		if expires != 0 {
			arg.ExpiresAt = pgtype.Timestamptz{Time: time.Now().Add(expires), Valid: true}
		}
		if _, err := l.q.CreateInvite(ctx, arg); err != nil {
			t.Fatal(err)
		}
		return raw
	}
	verifier := "a-verifier-that-is-long-enough-for-pkce-0123456789"
	sum := sha256.Sum256([]byte(verifier))
	// authorize pastes credential on the authorize page and returns the
	// status and, on success, the code.
	authorize := func(credential string) (int, string) {
		t.Helper()
		form := url.Values{
			"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {redirect},
			"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
			"api_key": {credential},
		}
		resp, err := l.client.PostForm(l.url+"/oauth/authorize", form)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		loc, _ := url.Parse(resp.Header.Get("Location"))
		return resp.StatusCode, loc.Query().Get("code")
	}

	raw := invite("ana", 0)
	status, code := authorize(raw)
	if status != http.StatusSeeOther || code == "" {
		t.Fatalf("redeem invite: %d", status)
	}
	if status, _ = authorize(raw); status != http.StatusBadRequest {
		t.Fatalf("an invite works once: %d", status)
	}
	if status, _ = authorize(auth.InvitePrefix + "not-an-invite"); status != http.StatusBadRequest {
		t.Fatalf("unknown invite: %d", status)
	}
	if status, _ = authorize(invite("late", -time.Minute)); status != http.StatusBadRequest {
		t.Fatalf("expired invite: %d", status)
	}

	resp, err := l.client.PostForm(l.url+"/oauth/token", url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"code_verifier": {verifier}, "client_id": {reg.ClientID}, "redirect_uri": {redirect}})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("token: %v %v", resp.StatusCode, err)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&tok)
	resp.Body.Close()
	var sp species.Detail
	l.do("POST", "/api/species", tok.AccessToken, map[string]any{"seed": 7}, http.StatusCreated, &sp)

	keys, err := l.q.ListAPIKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var minted string
	for _, k := range keys {
		if k.Name == "ana" {
			minted = k.ID
		}
	}
	if minted == "" {
		t.Fatal("redeeming the invite minted no key named after its note")
	}
	// The newcomer's key is a key like any other: deleting it cuts them off.
	if err := l.q.DeleteAPIKey(ctx, minted); err != nil {
		t.Fatal(err)
	}
	l.do("GET", "/api/export", tok.AccessToken, nil, http.StatusUnauthorized, nil)
}

func TestWebPages(t *testing.T) {
	l := newLab(t)
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, CheckRedirect: l.client.CheckRedirect}

	resp, _ := browser.Get(l.url + "/preview")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("signed-out preview: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, _ = browser.PostForm(l.url+"/login", url.Values{"api_key": {l.key}, "next": {"/preview"}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/preview" {
		t.Fatalf("login: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	page := func(path string) string {
		t.Helper()
		resp, err := browser.Get(l.url + path)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %v %v", path, resp.StatusCode, err)
		}
		if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'none'") {
			t.Fatalf("GET %s: no CSP", path)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return string(b)
	}
	if !strings.Contains(page("/preview?seed=5&count=2"), "Keep this one") {
		t.Fatal("preview page has no keep button")
	}

	seed := multiFormSeed(t)
	resp, _ = browser.PostForm(l.url+"/species", url.Values{"seed": {itoa(int(seed))}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("keep species: %d", resp.StatusCode)
	}
	speciesPath, _, _ := strings.Cut(resp.Header.Get("Location"), "?")
	plural := creaturelab.Roll(seed).Plural
	if !strings.Contains(page("/"), plural) {
		t.Fatal("catalog does not list the kept species")
	}
	resp, _ = browser.PostForm(l.url+speciesPath+"/forms/0/candidates", url.Values{"svg": {sprite}, "accept": {"1"}})
	if resp.StatusCode != http.StatusSeeOther || strings.Contains(resp.Header.Get("Location"), "err=") {
		t.Fatalf("paste candidate: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if body := page(speciesPath); !strings.Contains(body, "/sprites/") || !strings.Contains(body, "accepted") {
		t.Fatal("species page does not show the accepted sprite")
	}
	resp, _ = browser.Get(l.url + "/brief/" + strings.TrimPrefix(speciesPath, "/species/") + "/0")
	if b, _ := io.ReadAll(resp.Body); resp.StatusCode != http.StatusOK || !strings.Contains(string(b), creaturelab.HouseStyle) {
		t.Fatalf("brief as text: %d", resp.StatusCode)
	}

	// A cross-site form post is refused even with the cookie.
	req, _ := http.NewRequest("POST", l.url+speciesPath+"/delete", nil)
	req.Header.Set("Origin", "https://evil.example")
	if resp, _ = browser.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin delete: %d", resp.StatusCode)
	}

	// The ids the MCP tools take are on the pages, not only in the URL.
	speciesID := strings.TrimPrefix(speciesPath, "/species/")
	if !strings.Contains(page("/"), `<code class="id" title="Click to select. This is the id the MCP tools take.">`+speciesID+`</code>`) {
		t.Error("catalog does not show the species id")
	}
	signedIn := page(speciesPath)
	if !strings.Contains(signedIn, ">"+speciesID+"</code>") {
		t.Error("species page does not show its id")
	}
	cands, err := l.q.ListCandidates(context.Background(), speciesID)
	if err != nil || len(cands) == 0 || !strings.Contains(signedIn, ">"+cands[0].ID+"</code>") {
		t.Errorf("species page does not show its candidates' ids (%v)", err)
	}
	for _, want := range []string{"Save notes", "Paste an SVG", "Brief as text", "Remove", "Delete " + plural, "Sign out"} {
		if !strings.Contains(signedIn, want) {
			t.Errorf("signed-in species page lacks %q", want)
		}
	}

	resp, _ = browser.PostForm(l.url+"/logout", nil)
	if resp, _ = browser.Get(l.url + "/preview"); resp.StatusCode != http.StatusSeeOther {
		t.Fatal("signed out but the preview page still opens")
	}

	// Signed out, the catalog and species pages are public and read-only:
	// every sprite shows, and no form or button that changes anything does.
	// A stale cookie reads as signed out rather than locking them.
	jar.SetCookies(mustURL(l.url), []*http.Cookie{{Name: auth.SessionCookie, Value: "cl_at_stale"}})
	if body := page("/"); !strings.Contains(body, plural) || strings.Contains(body, "Roll and save") || !strings.Contains(body, "Sign in") {
		t.Fatal("signed-out catalog should list species read-only")
	}
	anon := page(speciesPath)
	if !strings.Contains(anon, "/sprites/") || !strings.Contains(anon, "accepted") {
		t.Fatal("signed-out species page does not show its sprites")
	}
	for _, hidden := range []string{"<form", "Paste an SVG", "Brief as text", "Remove", "Sign out"} {
		if strings.Contains(anon, hidden) {
			t.Errorf("signed-out species page shows %q", hidden)
		}
	}
	resp, _ = browser.PostForm(l.url+speciesPath+"/notes", url.Values{"notes": {"x"}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed-out notes post: %d", resp.StatusCode)
	}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

// bearer adds an Authorization header to every request.
type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

func TestMCPTools(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{
		Endpoint:   l.url + "/mcp/rpc",
		HTTPClient: &http.Client{Transport: bearer{l.key, http.DefaultTransport}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	call := func(name string, args map[string]any, out any) {
		t.Helper()
		res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s: tool error %+v", name, res.Content)
		}
		b, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	var previews struct{ Species []species.Preview }
	call("preview_species", map[string]any{"seed": 10, "count": 2}, &previews)
	if len(previews.Species) != 2 || previews.Species[1].Seed != 11 {
		t.Fatalf("preview: %+v", previews)
	}
	var created struct{ Species species.Detail }
	call("create_species", map[string]any{"seed": previews.Species[0].Seed}, &created)
	id := created.Species.ID

	var brief struct{ Brief string }
	call("get_sprite_brief", map[string]any{"speciesId": id, "form": 0}, &brief)
	if !strings.Contains(brief.Brief, creaturelab.HouseStyle) {
		t.Fatal("brief lacks the house style")
	}
	var sub struct{ Candidate sprites.Candidate }
	call("submit_sprite_candidate", map[string]any{"speciesId": id, "form": 0, "svg": sprite, "author": "test", "accept": true}, &sub)
	var got struct{ Species species.Detail }
	call("get_species", map[string]any{"id": id}, &got)
	if got.Species.Forms[0].AcceptedID != sub.Candidate.ID {
		t.Fatalf("form 0 accepted %q, want %q", got.Species.Forms[0].AcceptedID, sub.Candidate.ID)
	}

	res, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "submit_sprite_candidate",
		Arguments: map[string]any{"speciesId": id, "form": 0, "svg": "<script/>"}})
	if err != nil || !res.IsError {
		t.Fatalf("an unsafe SVG should come back as a tool error: %v %+v", err, res)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
