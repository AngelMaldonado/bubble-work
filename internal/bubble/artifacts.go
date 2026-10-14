package bubble

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"

	"github.com/AngelMaldonado/bubble-work/internal/md"
)

// Ephemeral artifacts: what gets thought through BEFORE the work exists.
//
// The first kind is the grill — rounds of structured questions an agent asks and
// a person answers, here or in their terminal. The model is general (`kind`);
// a new kind adds entry types and a validator to `kinds`, not collections.
//
// None of it is evidence. It lives outside git and outside the board, warms
// nothing, and is deleted `tuning.artifact_days` after it last moved unless
// somebody pins it. What is worth keeping goes into a thread at hand-off.

// ---- the pure part: what an entry may say, and what the entries add up to ----

// Entry is one append-only line of an artifact.
type Entry struct {
	Seq    int            `json:"seq"`
	Type   string         `json:"type"`
	Data   map[string]any `json:"data"`
	Author string         `json:"author,omitempty"`
	Via    string         `json:"via,omitempty"`
	At     string         `json:"at,omitempty"`
	// HTML is the note's text rendered — for the web only (see withHTML).
	HTML string `json:"html,omitempty"`
}

// kind is what one class of artifact accepts.
type kind struct {
	// validate checks one new entry against what is already there (prior) and
	// what comes before it in the same batch.
	validate func(e Entry, prior []Entry) error
}

var kinds = map[string]kind{
	"grill":    {validate: validateGrill},
	"markdown": {validate: validateMarkdown},
}

// Question is a grill question with everything derived about it.
type Question struct {
	ID             string   `json:"id"`
	Round          int      `json:"round"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	Choices        []string `json:"choices,omitempty"`
	Recommendation string   `json:"recommendation"`
	DependsOn      []string `json:"depends_on,omitempty"`
	Answer         *Answer  `json:"answer,omitempty"`
	// Rendered by the server for the web, with the engine every document goes
	// through. Empty over MCP: an agent reads the markdown it wrote.
	BodyHTML           string `json:"body_html,omitempty"`
	RecommendationHTML string `json:"recommendation_html,omitempty"`
	// answered · frontier (askable now: every dependency answered) · blocked
	State string `json:"state"`
}

// Answer is the latest answer a question got. Append-only: answering again
// replaces the verdict, not the row.
type Answer struct {
	Verdict string `json:"verdict"` // accepted | override
	Text    string `json:"text,omitempty"`
	Via     string `json:"via"`
	At      string `json:"at,omitempty"`
	HTML    string `json:"html,omitempty"`
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return strings.TrimSpace(s)
}

func strs(m map[string]any, k string) ([]string, error) {
	raw, ok := m[k]
	if !ok || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list of strings", k)
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be a list of strings", k)
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out, nil
}

func questionIDs(entries []Entry) map[string]bool {
	ids := map[string]bool{}
	for _, e := range entries {
		if e.Type == "question" {
			ids[str(e.Data, "id")] = true
		}
	}
	return ids
}

func validateGrill(e Entry, prior []Entry) error {
	ids := questionIDs(prior)
	switch e.Type {
	case "question":
		id := str(e.Data, "id")
		if id == "" || str(e.Data, "title") == "" || str(e.Data, "body") == "" {
			return fmt.Errorf("a question needs id, title and body")
		}
		if ids[id] {
			return fmt.Errorf("question %s already exists; a question is not rewritten, ask a new one", id)
		}
		if r, ok := e.Data["round"].(float64); !ok || r < 1 {
			return fmt.Errorf("question %s needs a round (1, 2, …)", id)
		}
		if _, err := strs(e.Data, "choices"); err != nil {
			return err
		}
		deps, err := strs(e.Data, "depends_on")
		if err != nil {
			return err
		}
		for _, d := range deps {
			if !ids[d] {
				return fmt.Errorf("question %s depends on %s, which was never asked", id, d)
			}
		}
	case "answer":
		q := str(e.Data, "question")
		if !ids[q] {
			return fmt.Errorf("there is no question %q to answer", q)
		}
		switch str(e.Data, "verdict") {
		case "accepted":
		case "override":
			if str(e.Data, "text") == "" {
				return fmt.Errorf("an override of %s needs the text that replaces the recommendation", q)
			}
		default:
			return fmt.Errorf("an answer's verdict is accepted or override")
		}
	case "note":
		if str(e.Data, "text") == "" {
			return fmt.Errorf("a note needs text")
		}
	default:
		return fmt.Errorf("a grill takes question, answer or note entries, not %q (hand-off is its own call)", e.Type)
	}
	return nil
}

// DeriveQuestions folds a grill's entries into its questions, each with its
// latest answer and its state. Nothing of this is stored: the tree and the
// frontier are a function of what was asked and answered.
func DeriveQuestions(entries []Entry) []Question {
	var qs []Question
	idx := map[string]int{}
	for _, e := range entries {
		switch e.Type {
		case "question":
			choices, _ := strs(e.Data, "choices")
			deps, _ := strs(e.Data, "depends_on")
			round, _ := e.Data["round"].(float64)
			idx[str(e.Data, "id")] = len(qs)
			qs = append(qs, Question{
				ID: str(e.Data, "id"), Round: int(round), Title: str(e.Data, "title"),
				Body: str(e.Data, "body"), Choices: choices,
				Recommendation: str(e.Data, "recommendation"), DependsOn: deps,
			})
		case "answer":
			if i, ok := idx[str(e.Data, "question")]; ok {
				qs[i].Answer = &Answer{
					Verdict: str(e.Data, "verdict"), Text: str(e.Data, "text"), Via: e.Via, At: e.At,
				}
			}
		}
	}
	for i := range qs {
		switch {
		case qs[i].Answer != nil:
			qs[i].State = "answered"
		default:
			qs[i].State = "frontier"
			for _, d := range qs[i].DependsOn {
				if j, ok := idx[d]; ok && qs[j].Answer == nil {
					qs[i].State = "blocked"
					break
				}
			}
		}
	}
	return qs
}

// ExpiresAt is when an unpinned artifact goes: N days after it last moved.
// Zero when pinned, which is what "never" looks like here.
func ExpiresAt(touched time.Time, pinned bool, days int) time.Time {
	if pinned {
		return time.Time{}
	}
	if days < 1 {
		days = 30
	}
	return touched.Add(time.Duration(days) * 24 * time.Hour)
}

// ---- the stored part ----

// agentLive is how recent an agent's last call has to be to count as connected.
// A token is stamped at most once a minute (tokens.go), and an agent waiting for
// a person to answer makes no calls at all, so the window is wider than that.
const agentLive = 5 * time.Minute

var errHandedOff = errors.New("this artifact was handed off; it is read-only now")

func canTouchArtifact(app core.App, auth, art *core.Record) bool {
	if !isPersonAuth(auth) {
		return false
	}
	if art.GetString("creator") == auth.Id {
		return true
	}
	// Un lead del equipo de la idea de la que cuelga.
	if item, err := app.FindRecordById("inbox_items", art.GetString("inbox_item")); err == nil {
		return LeadsTeam(app, auth, item.GetString("team"))
	}
	return false
}

func artifactFor(app core.App, auth *core.Record, id string) (*core.Record, error) {
	art, err := app.FindRecordById("artifacts", id)
	if err != nil || !canTouchArtifact(app, auth, art) {
		return nil, ErrDenied
	}
	return art, nil
}

func artifactDays(app core.App) int {
	rows, err := app.FindAllRecords("tuning")
	if err != nil || len(rows) == 0 {
		return 30
	}
	if d := rows[0].GetInt("artifact_days"); d > 0 {
		return d
	}
	return 30
}

func loadEntries(app core.App, artID string) ([]Entry, error) {
	rows, err := app.FindRecordsByFilter("artifact_entries", "artifact = {:a}", "seq", 0, 0, dbx.Params{"a": artID})
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		var data map[string]any
		_ = json.Unmarshal([]byte(r.GetString("data")), &data)
		out = append(out, Entry{
			Seq: r.GetInt("seq"), Type: r.GetString("type"), Data: data,
			Author: r.GetString("author"), Via: r.GetString("via"), At: r.GetString("created"),
		})
	}
	return out, nil
}

// ArtifactStart opens an artifact on an inbox item — an existing one, or a new
// one made from `topic`. The inbox item is the topic and the backlog row; it
// outlives the artifact.
func ArtifactStart(app core.App, auth *core.Record, kindName, inboxItem, topic, team string) (*core.Record, error) {
	if !isPersonAuth(auth) {
		return nil, ErrNotAPerson
	}
	if _, ok := kinds[kindName]; !ok {
		return nil, fmt.Errorf("no artifact kind %q (there are: grill, markdown)", kindName)
	}
	topic = strings.TrimSpace(topic)
	var item *core.Record
	if inboxItem != "" {
		r, err := app.FindRecordById("inbox_items", inboxItem)
		if err != nil || (r.GetString("captured_by") != auth.Id && !LeadsTeam(app, auth, r.GetString("team"))) {
			return nil, ErrDenied
		}
		item = r
	} else {
		if topic == "" {
			return nil, fmt.Errorf("say what it is about: a topic, or an inbox item")
		}
		r, err := Capture(app, auth, team, topic)
		if err != nil {
			return nil, err
		}
		item = r
	}
	title := topic
	if title == "" {
		title = item.GetString("note")
	}
	if len([]rune(title)) > 300 {
		title = string([]rune(title)[:300])
	}
	col, err := app.FindCollectionByNameOrId("artifacts")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	art := core.NewRecord(col)
	art.Set("kind", kindName)
	art.Set("title", title)
	art.Set("inbox_item", item.Id)
	art.Set("creator", auth.Id)
	art.Set("status", "open")
	art.Set("touched_at", now)
	art.Set("agent_at", now)
	art.Set("watch_key", security.RandomString(40))
	if err := app.Save(art); err != nil {
		return nil, err
	}
	return art, nil
}

// ArtifactAppend adds entries in order. `via` is "harness" for an agent and
// "app" for the web; an answer the person gave in the terminal is written by the
// agent with via=harness, so the record has no holes.
func ArtifactAppend(app core.App, auth *core.Record, id string, entries []Entry, via string) ([]Entry, error) {
	art, err := artifactFor(app, auth, id)
	if err != nil {
		return nil, err
	}
	if art.GetString("status") != "open" {
		return nil, errHandedOff
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("nothing to append")
	}
	if art.GetString("kind") == "markdown" && via != "harness" {
		return nil, ErrDenied // documents are published by the agent, never edited in the web
	}
	k := kinds[art.GetString("kind")]
	var written []Entry
	err = app.RunInTransaction(func(tx core.App) error {
		// Recheck under the write transaction: a handoff may have committed
		// while this publisher was waiting for the transaction lock.
		art, err = artifactFor(tx, auth, id)
		if err != nil {
			return err
		}
		if art.GetString("status") != "open" {
			return errHandedOff
		}
		prior, err := loadEntries(tx, art.Id)
		if err != nil {
			return err
		}
		next := 1
		if len(prior) > 0 {
			next = prior[len(prior)-1].Seq + 1
		}
		col, err := tx.FindCollectionByNameOrId("artifact_entries")
		if err != nil {
			return err
		}
		for i, e := range entries {
			e.Type = strings.TrimSpace(e.Type)
			if e.Data == nil {
				e.Data = map[string]any{}
			}
			if err := k.validate(e, prior); err != nil {
				return fmt.Errorf("entry %d: %w", i+1, err)
			}
			e.Seq, e.Author, e.Via = next, auth.Id, via
			r := core.NewRecord(col)
			r.Set("artifact", art.Id)
			r.Set("seq", e.Seq)
			r.Set("type", e.Type)
			r.Set("data", e.Data)
			r.Set("author", auth.Id)
			r.Set("via", via)
			if err := tx.Save(r); err != nil {
				return err
			}
			e.At = r.GetString("created")
			prior = append(prior, e)
			written = append(written, e)
			next++
		}
		now := time.Now().UTC()
		art.Set("touched_at", now)
		if via == "harness" {
			art.Set("agent_at", now)
		}
		return tx.Save(art)
	})
	return written, err
}

// ArtifactView is an artifact as both doors show it.
type ArtifactView struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Pinned    bool   `json:"pinned"`
	InboxItem string `json:"inbox_item"`
	Creator   string `json:"creator"`
	TouchedAt string `json:"touched_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Live      bool   `json:"live"`
	// An agent is blocked right now waiting for this person to answer.
	Watched bool   `json:"watched"`
	URL     string `json:"url,omitempty"`
	// What an agent with a shell runs to be told when the person presses
	// «Enviar». Only over MCP: it carries the watch key.
	WatchCommand string            `json:"watch_command,omitempty"`
	Questions    []Question        `json:"questions,omitempty"`
	Total        int               `json:"total"`
	Answered     int               `json:"answered"`
	Waiting      int               `json:"waiting"`
	Entries      []Entry           `json:"entries,omitempty"`
	Document     *ArtifactDocument `json:"document,omitempty"`
}

// BaseURL is where this server is reached, as the caller reached it: the agent
// that called /mcp can reach that address, which is the one its script needs.
// Behind a proxy the forwarded headers say it; AppURL is the fallback.
func BaseURL(app core.App, r *http.Request) string {
	if r != nil && r.Host != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
			scheme = strings.TrimSpace(strings.Split(p, ",")[0])
		}
		host := r.Host
		if h := r.Header.Get("X-Forwarded-Host"); h != "" {
			host = strings.TrimSpace(strings.Split(h, ",")[0])
		}
		return scheme + "://" + host
	}
	return strings.TrimRight(app.Settings().Meta.AppURL, "/")
}

// WithLinks adds what only the agent's door carries: the link to give the
// person, and the command that waits for their answers. `since` is the last
// seq the agent has seen, so the command does not wake it for old answers.
func WithLinks(app core.App, v ArtifactView, base string) ArtifactView {
	base = strings.TrimRight(base, "/")
	v.URL = base + "/artefacto/" + v.ID
	art, err := app.FindRecordById("artifacts", v.ID)
	if err != nil || art.GetString("watch_key") == "" || v.Status != "open" || v.Kind != "grill" {
		return v
	}
	since := 0
	if entries, err := loadEntries(app, v.ID); err == nil && len(entries) > 0 {
		since = entries[len(entries)-1].Seq
	}
	v.WatchCommand = watchCommand(base, v.ID, art.GetString("watch_key"), since)
	return v
}

// watchCommand prints ONE line per «Enviar» and nothing else — under Claude
// Code's Monitor every line wakes the agent, so there is no heartbeat. It asks
// again every 55 s so a proxy never sees an idle connection, backs off on an
// error instead of hammering, and exits when the artifact is handed off or gone.
func watchCommand(base, id, key string, since int) string {
	url := fmt.Sprintf("%s/api/artifacts/%s/wait?key=%s", base, id, key)
	return fmt.Sprintf(`S=%d; while :; do r=$(curl -s --max-time 60 -w '|HTTP:%%{http_code}' "%s&since=$S") || { sleep 5; continue; }; c=${r##*|HTTP:}; r=$(printf '%%s' "${r%%|HTTP:*}"); case "$c" in 404) echo "fin: el artefacto ya no está disponible seq=$S"; exit 0;; 204) continue;; 200) ;; *) sleep 5; continue;; esac; [ -z "$r" ] && continue; echo "$r"; case "$r" in fin*) exit 0;; esac; S=${r##*seq=}; done`, since, url)
}

// ---- waiting for the person ----

// waiters remembers which artifacts an agent is blocked on right now, so the
// UI can say «tu agente está esperando» rather than guess. In memory: it is a
// fact about this process's open connections, and a restart ends them anyway.
var waiters sync.Map // artifact id -> time.Time of the last wait call

const watchedFor = 75 * time.Second // > one wait call plus the script's backoff

func isWatched(id string) bool {
	if t, ok := waiters.Load(id); ok {
		return time.Since(t.(time.Time)) < watchedFor
	}
	return false
}

// Answered is what a wait reports: the questions answered in the app after
// `since`, and the seq to wait from next. Closed means handed off or gone.
type Answered struct {
	Questions []string `json:"answered,omitempty"`
	Seq       int      `json:"seq"`
	Closed    bool     `json:"closed,omitempty"`
}

// WaitAnswers blocks until the person answers in the app (one «Enviar» is one
// batch), the artifact closes, `max` passes, or ctx ends. Answers recorded by
// an agent from the terminal do not count: that agent already knows them.
func WaitAnswers(ctx context.Context, app core.App, id string, since int, max time.Duration) (Answered, error) {
	waiters.Store(id, time.Now())
	defer waiters.Store(id, time.Now())
	deadline := time.Now().Add(max)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		art, err := app.FindRecordById("artifacts", id)
		if err != nil {
			return Answered{Closed: true, Seq: since}, nil
		}
		entries, err := loadEntries(app, id)
		if err != nil {
			return Answered{}, err
		}
		out := Answered{Seq: since}
		seen := map[string]bool{}
		for _, e := range entries {
			if e.Seq <= since {
				continue
			}
			out.Seq = e.Seq
			if e.Type == "answer" && e.Via == "app" {
				if q := str(e.Data, "question"); !seen[q] {
					seen[q] = true
					out.Questions = append(out.Questions, q)
				}
			}
		}
		if art.GetString("status") != "open" {
			out.Closed = true
			return out, nil
		}
		if len(out.Questions) > 0 {
			return out, nil
		}
		if time.Now().After(deadline) {
			return Answered{Seq: since}, nil
		}
		select {
		case <-ctx.Done():
			return Answered{Seq: since}, nil
		case <-tick.C:
			waiters.Store(id, time.Now())
		}
	}
}

func waitSeconds(raw string, def, ceiling int) time.Duration {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		n = def
	}
	if n > ceiling {
		n = ceiling
	}
	return time.Duration(n) * time.Second
}

func viewOf(app core.App, art *core.Record, entries []Entry, withEntries bool) ArtifactView {
	qs := DeriveQuestions(entries)
	v := ArtifactView{
		ID: art.Id, Kind: art.GetString("kind"), Title: art.GetString("title"),
		Status: art.GetString("status"), Pinned: art.GetBool("pinned"),
		InboxItem: art.GetString("inbox_item"), Creator: art.GetString("creator"),
		TouchedAt: art.GetString("touched_at"), Watched: isWatched(art.Id),
	}
	if t := ExpiresAt(art.GetDateTime("touched_at").Time(), v.Pinned, artifactDays(app)); !t.IsZero() {
		v.ExpiresAt = t.UTC().Format(time.RFC3339)
	}
	if a := art.GetDateTime("agent_at"); !a.IsZero() && time.Since(a.Time()) < agentLive && v.Status == "open" {
		v.Live = true
	}
	v.Total = len(qs)
	for _, q := range qs {
		switch q.State {
		case "answered":
			v.Answered++
		case "frontier":
			v.Waiting++
		}
	}
	if v.Kind == "markdown" {
		v.Document = latestDocument(entries)
		if !withEntries && v.Document != nil {
			v.Document.Markdown = ""
		}
	}
	if withEntries {
		v.Questions = qs
		v.Entries = entries
	}
	return v
}

// ArtifactRead returns an artifact with its derived questions. `since` keeps
// only entries after that seq, for an agent catching up between rounds; the
// questions are always derived from the whole record.
func ArtifactRead(app core.App, auth *core.Record, id string, since int) (ArtifactView, error) {
	art, err := artifactFor(app, auth, id)
	if err != nil {
		return ArtifactView{}, err
	}
	entries, err := loadEntries(app, art.Id)
	if err != nil {
		return ArtifactView{}, err
	}
	v := viewOf(app, art, entries, true)
	if since > 0 {
		var tail []Entry
		for _, e := range entries {
			if e.Seq > since {
				tail = append(tail, e)
			}
		}
		v.Entries = tail
	}
	return v, nil
}

// withHTML renders every markdown field of a view for the WEB, through the same
// goldmark the threads use (`md.RenderHTML`): one engine, so a question reads
// the way a document does — and mermaid, excalidraw and folded images come from
// `Prose` on top, like everywhere else. MCP never calls this; an agent gets its
// own markdown back, not a second copy of it in HTML.
func withHTML(v ArtifactView) ArtifactView {
	if v.Document != nil {
		doc := *v.Document
		doc.HTML = md.RenderHTML(doc.Markdown)
		v.Document = &doc
	}
	for i := range v.Questions {
		q := &v.Questions[i]
		q.BodyHTML = md.RenderHTML(q.Body)
		q.RecommendationHTML = md.RenderHTML(q.Recommendation)
		if q.Answer != nil && q.Answer.Text != "" {
			a := *q.Answer
			a.HTML = md.RenderHTML(a.Text)
			q.Answer = &a
		}
	}
	for i := range v.Entries {
		if v.Entries[i].Type == "note" {
			v.Entries[i].HTML = md.RenderHTML(str(v.Entries[i].Data, "text"))
		}
	}
	return v
}

// Target is where the work went at hand-off.
type Target struct {
	Kind string `json:"kind"` // thread | bubble
	ID   string `json:"id"`
}

// ArtifactHandoff closes the thinking: the work now lives in the targets, which
// the agent has ALREADY written so they stand on their own (decisions with what
// they rejected, open questions, first steps). No link back — the artifact will
// be deleted. The inbox item is triaged to the first thread and first bubble.
func ArtifactHandoff(app core.App, auth *core.Record, id string, targets []Target) (ArtifactView, error) {
	art, err := artifactFor(app, auth, id)
	if err != nil {
		return ArtifactView{}, err
	}
	if art.GetString("status") != "open" {
		return ArtifactView{}, errHandedOff
	}
	if len(targets) == 0 {
		return ArtifactView{}, fmt.Errorf("say where the work went: at least one thread or bubble")
	}
	var thread, bubbleID string
	for _, t := range targets {
		switch t.Kind {
		case "thread":
			if _, _, _, err := ThreadFor(app, auth, t.ID); err != nil {
				return ArtifactView{}, fmt.Errorf("thread %q: %w", t.ID, err)
			}
			if thread == "" {
				thread = t.ID
			}
		case "bubble":
			b, err := app.FindRecordById("bubbles", t.ID)
			if err != nil {
				return ArtifactView{}, fmt.Errorf("no bubble %q", t.ID)
			}
			if _, _, err := WorkspaceFor(app, auth, b.GetString("workspace")); err != nil {
				return ArtifactView{}, fmt.Errorf("bubble %q: %w", t.ID, err)
			}
			if bubbleID == "" {
				bubbleID = t.ID
			}
		default:
			return ArtifactView{}, fmt.Errorf("a target is a thread or a bubble, not %q", t.Kind)
		}
	}
	err = app.RunInTransaction(func(tx core.App) error {
		prior, err := loadEntries(tx, art.Id)
		if err != nil {
			return err
		}
		next := 1
		if len(prior) > 0 {
			next = prior[len(prior)-1].Seq + 1
		}
		col, err := tx.FindCollectionByNameOrId("artifact_entries")
		if err != nil {
			return err
		}
		r := core.NewRecord(col)
		r.Set("artifact", art.Id)
		r.Set("seq", next)
		r.Set("type", "handoff")
		r.Set("data", map[string]any{"targets": targets})
		r.Set("author", auth.Id)
		r.Set("via", "harness")
		if err := tx.Save(r); err != nil {
			return err
		}
		art.Set("status", "handed_off")
		art.Set("touched_at", time.Now().UTC()) // the clock restarts at hand-off
		if err := tx.Save(art); err != nil {
			return err
		}
		item, err := tx.FindRecordById("inbox_items", art.GetString("inbox_item"))
		if err != nil {
			return nil // the note is gone; the hand-off still stands
		}
		if thread != "" && item.GetString("thread") == "" {
			item.Set("thread", thread)
		}
		if bubbleID != "" && item.GetString("bubble") == "" {
			item.Set("bubble", bubbleID)
		}
		return tx.Save(item)
	})
	if err != nil {
		return ArtifactView{}, err
	}
	return ArtifactRead(app, auth, id, 0)
}

// ArtifactDelete removes the ephemeral record and its entries, never the inbox
// note or durable work. Recheck access inside the same transaction as deletion.
func ArtifactDelete(app core.App, auth *core.Record, id string) error {
	return app.RunInTransaction(func(tx core.App) error {
		art, err := artifactFor(tx, auth, id)
		if err != nil {
			return err
		}
		return tx.Delete(art)
	})
}

// ArtifactPin fixes an artifact so it never expires, or lets it go again —
// which restarts its clock, so unpinning is not a delete by surprise.
func ArtifactPin(app core.App, auth *core.Record, id string, pinned bool) error {
	art, err := artifactFor(app, auth, id)
	if err != nil {
		return err
	}
	art.Set("pinned", pinned)
	if !pinned {
		art.Set("touched_at", time.Now().UTC())
	}
	return app.Save(art)
}

// sweepArtifacts deletes what expired. Hard delete: none of it is in git, and
// its entries go with it (cascade). The inbox item stays.
func sweepArtifacts(app core.App, now time.Time) int {
	days := artifactDays(app)
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour).UTC()
	rows, err := app.FindRecordsByFilter("artifacts", "pinned = false && touched_at < {:c}", "", 0, 0,
		dbx.Params{"c": cutoff.Format("2006-01-02 15:04:05.000Z")})
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range rows {
		if err := app.Delete(r); err == nil {
			n++
		}
	}
	return n
}

// agentStatus says whether one of this person's agents called the MCP lately.
func agentStatus(app core.App, auth *core.Record) map[string]any {
	rows, err := app.FindRecordsByFilter("agent_tokens", "owner = {:o} && revoked_at = ''", "-last_used_at", 1, 0,
		dbx.Params{"o": auth.Id})
	out := map[string]any{"live": false, "last_used_at": ""}
	if err != nil || len(rows) == 0 {
		return out
	}
	last := rows[0].GetDateTime("last_used_at")
	if last.IsZero() {
		return out
	}
	out["last_used_at"] = last.Time().UTC().Format(time.RFC3339)
	out["live"] = time.Since(last.Time()) < agentLive
	return out
}

func registerArtifacts(app core.App) {
	app.Cron().MustAdd("bubbleArtifactsExpire", "17 * * * *", func() {
		if n := sweepArtifacts(app, time.Now()); n > 0 {
			app.Logger().Info("artifacts expired", "count", n)
		}
	})

	fail := func(e *core.RequestEvent, err error) error {
		if errors.Is(err, ErrDenied) {
			return e.NotFoundError("not found", nil)
		}
		return e.BadRequestError(err.Error(), nil)
	}

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		// The brain lists mine. The planner's inbox asks for the department's
		// artifacts so its TTL badges include the team's; only a global lead
		// can request that scope.
		se.Router.GET("/api/artifacts", func(e *core.RequestEvent) error {
			if !isPersonAuth(e.Auth) {
				return e.UnauthorizedError("sign in first", nil)
			}
			all := e.Request.URL.Query().Get("scope") == "inbox"
			if all && len(LedTeams(e.App, e.Auth)) == 0 {
				return e.ForbiddenError("only a team lead reads the team's inbox", nil)
			}
			list, err := ArtifactsOf(e.App, e.Auth, all)
			if err != nil {
				return e.InternalServerError(err.Error(), err)
			}
			return e.JSON(http.StatusOK, map[string]any{
				"agent": agentStatus(e.App, e.Auth), "artifacts": list,
			})
		}).Bind(apis.RequireAuth())

		// What a waiting agent's script asks. The watch key IS the credential —
		// the script cannot hold the agent's token — and it opens only this:
		// one line when the person presses «Enviar», nothing else. 204 when the
		// wait times out, so the script just asks again.
		se.Router.GET("/api/artifacts/{id}/wait", func(e *core.RequestEvent) error {
			id := e.Request.PathValue("id")
			art, err := e.App.FindRecordById("artifacts", id)
			key := e.Request.URL.Query().Get("key")
			if err != nil || key == "" ||
				subtle.ConstantTimeCompare([]byte(key), []byte(art.GetString("watch_key"))) != 1 {
				return e.NotFoundError("not found", nil)
			}
			since, _ := strconv.Atoi(e.Request.URL.Query().Get("since"))
			got, err := WaitAnswers(e.Request.Context(), e.App, id, since,
				waitSeconds(e.Request.URL.Query().Get("timeout"), 50, 55))
			if err != nil {
				return e.InternalServerError(err.Error(), err)
			}
			switch {
			case got.Closed:
				return e.String(http.StatusOK, fmt.Sprintf("fin: el artefacto se cerró seq=%d\n", got.Seq))
			case len(got.Questions) > 0:
				return e.String(http.StatusOK, fmt.Sprintf(
					"respondidas %s — llama artifact_read(%q) seq=%d\n",
					strings.Join(got.Questions, ","), id, got.Seq))
			default:
				return e.NoContent(http.StatusNoContent)
			}
		})

		se.Router.GET("/api/artifacts/{id}", func(e *core.RequestEvent) error {
			v, err := ArtifactRead(e.App, e.Auth, e.Request.PathValue("id"), 0)
			if err != nil {
				return fail(e, err)
			}
			return e.JSON(http.StatusOK, withHTML(v))
		}).Bind(apis.RequireAuth())

		se.Router.DELETE("/api/artifacts/{id}", func(e *core.RequestEvent) error {
			if err := ArtifactDelete(e.App, e.Auth, e.Request.PathValue("id")); err != nil {
				return fail(e, err)
			}
			return e.NoContent(http.StatusNoContent)
		}).Bind(apis.RequireAuth())

		// Answers from the web. Only answers: asking is the agent's.
		se.Router.POST("/api/artifacts/{id}/answers", func(e *core.RequestEvent) error {
			var body struct {
				Answers []struct {
					Question string `json:"question"`
					Verdict  string `json:"verdict"`
					Text     string `json:"text"`
				} `json:"answers"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.BadRequestError("could not read the body", err)
			}
			entries := make([]Entry, 0, len(body.Answers))
			for _, a := range body.Answers {
				entries = append(entries, Entry{Type: "answer", Data: map[string]any{
					"question": a.Question, "verdict": a.Verdict, "text": a.Text,
				}})
			}
			if _, err := ArtifactAppend(e.App, e.Auth, e.Request.PathValue("id"), entries, "app"); err != nil {
				return fail(e, err)
			}
			v, err := ArtifactRead(e.App, e.Auth, e.Request.PathValue("id"), 0)
			if err != nil {
				return fail(e, err)
			}
			return e.JSON(http.StatusOK, withHTML(v))
		}).Bind(apis.RequireAuth())

		se.Router.POST("/api/artifacts/{id}/pin", func(e *core.RequestEvent) error {
			var body struct {
				Pinned bool `json:"pinned"`
			}
			if err := e.BindBody(&body); err != nil {
				return e.BadRequestError("could not read the body", err)
			}
			if err := ArtifactPin(e.App, e.Auth, e.Request.PathValue("id"), body.Pinned); err != nil {
				return fail(e, err)
			}
			v, err := ArtifactRead(e.App, e.Auth, e.Request.PathValue("id"), 0)
			if err != nil {
				return fail(e, err)
			}
			return e.JSON(http.StatusOK, withHTML(v))
		}).Bind(apis.RequireAuth())
		return se.Next()
	})
}
