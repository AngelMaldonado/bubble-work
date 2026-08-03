package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/AngelMaldonado/bubble-work/internal/domain"
	"github.com/AngelMaldonado/bubble-work/internal/md"
	"github.com/AngelMaldonado/bubble-work/internal/plane"
)

// planeAssetImgRe matches the <img> tags md emits for Plane image-components
// (their src carries the plane-asset: marker). Regular <img>/![](url) with real
// URLs are left untouched so they render normally.
var planeAssetImgRe = regexp.MustCompile(`<img\b[^>]*\bsrc="` + regexp.QuoteMeta(md.AssetScheme) + `[^"]*"[^>]*>`)

// rewriteAssets replaces Plane description-image markers with an "open in Plane"
// link. Plane serves these assets only to a web session (the API key is rejected
// on every asset endpoint), so we can't render them inline; the link opens the
// item in Plane where they display. Real image URLs are left as <img>.
func rewriteAssets(htmlStr, itemURL string) string {
	if !strings.Contains(htmlStr, md.AssetScheme) {
		return htmlStr
	}
	link := `<a class="plane-img" href="` + itemURL + `" target="_blank" rel="noopener">🖼 image — open in Plane</a>`
	return planeAssetImgRe.ReplaceAllStringFunc(htmlStr, func(string) string { return link })
}

// planeItemURL builds the Plane web URL for a work item.
func planeItemURL(base, ws, proj, id string) string {
	return strings.TrimRight(base, "/") + "/" + ws + "/projects/" + proj + "/issues/" + id
}

// guard wraps a task run in an errgroup goroutine so a panic becomes an error
// instead of crashing the whole process (a panic in a spawned goroutine is NOT
// recovered by net/http).
func guard(name string, fn func() error) func() error {
	return func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("interior: %s panicked: %v", name, r)
				err = fmt.Errorf("%s failed", name)
			}
		}()
		return fn()
	}
}

// interiorClient parses a namespaced id (slug:project:objectid), authorizes the
// actor for that instance (read = workspace membership), and returns a Plane
// client pinned to the project plus the id parts.
func (s *Server) interiorClient(ctx context.Context, nsID string) (cl *plane.Client, inst domain.Instance, slug, projID, objID string, err error) {
	parts := strings.SplitN(nsID, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, domain.Instance{}, "", "", "", errNotFound
	}
	slug, projID, objID = parts[0], parts[1], parts[2]

	actor, _ := domain.ActorFrom(ctx)
	if !actor.CanSee(slug) {
		return nil, domain.Instance{}, "", "", "", errForbid
	}
	inst, ok, err := s.instanceBySlug(slug)
	if err != nil {
		return nil, domain.Instance{}, "", "", "", err
	}
	if !ok {
		return nil, domain.Instance{}, "", "", "", errNotFound
	}
	cl = plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, projID)
	return cl, inst, slug, projID, objID, nil
}

// memberNames maps an instance's member ids to display names for resolving
// assignees and comment authors, cached for membersTTL (members change rarely
// and this call otherwise runs on every timeline/thread open). Best-effort: an
// error serves a stale map if we have one, else empty.
func (s *Server) memberNames(ctx context.Context, inst domain.Instance) map[string]string {
	now := s.now()
	s.membersMu.Lock()
	if c, ok := s.membersCache[inst.Slug]; ok && now.Before(c.exp) {
		s.membersMu.Unlock()
		return c.names
	}
	s.membersMu.Unlock()

	members, err := plane.New(inst.BaseURL, inst.APIKey, inst.Workspace, "").Members(ctx)
	if err != nil {
		s.membersMu.Lock()
		defer s.membersMu.Unlock()
		if c, ok := s.membersCache[inst.Slug]; ok {
			return c.names // stale is better than nameless
		}
		return map[string]string{}
	}
	names := make(map[string]string, len(members))
	for _, m := range members {
		if m.DisplayName != "" {
			names[m.ID] = m.DisplayName
		} else {
			names[m.ID] = m.Email
		}
	}
	s.membersMu.Lock()
	s.membersCache[inst.Slug] = cachedMembers{names: names, exp: now.Add(membersTTL)}
	s.membersMu.Unlock()
	return names
}

// Timeline returns a bubble's threads newest-first (INTERIOR-PLAN.md Phase 10),
// served straight from the in-memory snapshot — no Plane fetch on the request
// path. bubbleID may be a short id or a full namespaced id (resolved like heat).
func (s *Server) Timeline(ctx context.Context, bubbleID string) ([]domain.ThreadNode, error) {
	full, err := s.resolveID(ctx, bubbleID)
	if err != nil {
		return nil, err
	}
	bubbles, err := s.collect(ctx)
	if err != nil {
		return nil, err
	}
	var b *domain.Bubble
	for i := range bubbles {
		if bubbles[i].ID == full {
			b = &bubbles[i]
			break
		}
	}
	if b == nil {
		return nil, errNotFound
	}
	slug, projID, _ := cut3(b.ID)

	out := make([]domain.ThreadNode, 0, len(b.Threads))
	for _, t := range b.Threads {
		out = append(out, domain.ThreadNode{
			ID:          slug + ":" + projID + ":" + t.ID,
			Seq:         t.Seq,
			Title:       t.Name,
			Active:      t.Active,
			Owner:       t.Owner,
			Parent:      t.Parent,
			CreatedAt:   t.CreatedAt,
			CompletedAt: t.CompletedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ThreadDetail returns a thread's full interior (INTERIOR-PLAN.md Phase 11).
// threadID may be a full namespaced id (slug:project:workitem) or a bare
// work-item id/prefix, resolved against the caller's threads.
func (s *Server) ThreadDetail(ctx context.Context, threadID string) (domain.ThreadDetail, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	cl, inst, _, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.ThreadDetail{}, err
	}

	// Serve from cache when fresh — building the detail pages the whole project
	// for revisions (~1.7s). singleflight coalesces concurrent misses.
	s.detailMu.Lock()
	if c, ok := s.detailCache[wid]; ok && s.now().Before(c.exp) {
		s.detailMu.Unlock()
		return c.detail, nil
	}
	s.detailMu.Unlock()

	v, err, _ := s.sf.Do("detail:"+wid, func() (any, error) {
		d, e := s.buildThreadDetail(ctx, cl, inst, projID, wid, full)
		if e != nil {
			return nil, e
		}
		s.detailMu.Lock()
		s.detailCache[wid] = cachedDetail{detail: d, exp: s.now().Add(detailTTL)}
		s.detailMu.Unlock()
		return d, nil
	})
	if err != nil {
		return domain.ThreadDetail{}, err
	}
	return v.(domain.ThreadDetail), nil
}

// buildThreadDetail renders a thread's interior with live Plane fetches. Callers
// go through ThreadDetail, which caches the (viewer-agnostic) result.
func (s *Server) buildThreadDetail(ctx context.Context, cl *plane.Client, inst domain.Instance, projID, wid, full string) (domain.ThreadDetail, error) {
	// Fetch the body, revisions, and member names concurrently — each is a
	// separate Plane round-trip (~0.3-1s), so serial would stack up.
	var (
		wi        plane.WorkItemDetail
		revisions []md.Artifact
		names     map[string]string
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(guard("get-work-item", func() error {
		var e error
		wi, e = cl.GetWorkItem(gctx, wid)
		return e
	}))
	g.Go(guard("revisions", func() error {
		revisions = s.revisions(gctx, cl, wid)
		return nil
	}))
	g.Go(guard("members", func() error {
		names = s.memberNames(gctx, inst)
		return nil
	}))
	if err := g.Wait(); err != nil {
		return domain.ThreadDetail{}, err
	}

	bodyMD := md.FromHTML(wi.DescriptionHTML)
	arts, log := md.ParseThread(bodyMD, wi.Name)
	kind := "simple"
	if log != nil && log.Phased {
		kind = "phased"
	}

	var assignees []string
	for _, a := range wi.Assignees {
		if n := names[a]; n != "" {
			assignees = append(assignees, n)
		}
	}

	// Replace Plane image markers with an "open in Plane" link (revisions are
	// rewritten in revisions() where each has its own item id).
	itemURL := planeItemURL(inst.BaseURL, inst.Workspace, projID, wid)
	for i := range arts {
		arts[i].HTML = rewriteAssets(arts[i].HTML, itemURL)
	}

	// Normalize to non-nil slices so the JSON is arrays, never null (the web
	// client indexes/`.length`s them).
	if arts == nil {
		arts = []md.Artifact{}
	}
	if revisions == nil {
		revisions = []md.Artifact{}
	}
	if log != nil && log.Todos == nil {
		log.Todos = []md.Todo{}
	}

	return domain.ThreadDetail{
		ID:          full,
		Seq:         wi.Sequence,
		Title:       wi.Name,
		Kind:        kind,
		Active:      wi.CompletedAt == nil,
		Priority:    wi.Priority,
		Assignees:   assignees,
		Artifacts:   arts,
		Logbook:     log,
		Revisions:   revisions,
		CreatedAt:   wi.CreatedAt,
		CompletedAt: wi.CompletedAt,
	}, nil
}

// resolveThreadID normalizes a thread query to a full namespaced id
// (slug:project:workitem). A query already carrying the two colons is returned
// as-is; a bare work-item id (or unique prefix) is located among the caller's
// threads so `bubble thread <wid>` works like `heat`.
func (s *Server) resolveThreadID(ctx context.Context, q string) (string, error) {
	if strings.Count(q, ":") >= 2 {
		return q, nil
	}
	bubbles, err := s.collect(ctx)
	if err != nil {
		return "", err
	}
	var hits []string
	for _, b := range bubbles {
		slug, projID, _ := cut3(b.ID)
		for _, t := range b.Threads {
			ns := slug + ":" + projID + ":" + t.ID
			if t.ID == q {
				return ns, nil // exact wid wins outright
			}
			if strings.HasPrefix(t.ID, q) {
				hits = append(hits, ns)
			}
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return "", errNotFound
	default:
		return "", fmt.Errorf("%w: %q matches %d threads — use more characters", errAmbig, q, len(hits))
	}
}

// cut3 splits a namespaced bubble id (slug:project:module) into its parts.
func cut3(id string) (slug, proj, obj string) {
	parts := strings.SplitN(id, ":", 3)
	for i := range parts {
		switch i {
		case 0:
			slug = parts[0]
		case 1:
			proj = parts[1]
		case 2:
			obj = parts[2]
		}
	}
	return slug, proj, obj
}

// revisions renders a thread's sub-work-items (children) as free-form revision
// artifacts. Plane's relations API is unavailable on some self-hosted instances,
// so revisions attach via the parent/child link instead; an optional "rev:" name
// prefix is stripped for the label. The child list already carries
// description_html, so no per-item fetch is needed. Best-effort: never fatal.
func (s *Server) revisions(ctx context.Context, cl *plane.Client, wid string) []md.Artifact {
	children, err := cl.ListChildren(ctx, wid)
	if err != nil || len(children) == 0 {
		return nil
	}
	out := make([]md.Artifact, 0, len(children))
	for _, ch := range children {
		label := strings.TrimSpace(ch.Name)
		if len(label) >= 4 && strings.EqualFold(label[:4], "rev:") {
			if rest := strings.TrimSpace(label[4:]); rest != "" {
				label = rest
			}
		}
		art := md.NewArtifact(label, md.FromHTML(ch.DescriptionHTML))
		art.HTML = rewriteAssets(art.HTML, planeItemURL(cl.BaseURL, cl.Workspace, cl.Project, ch.ID))
		out = append(out, art)
	}
	return out
}

// ThreadComments returns a thread's comment feed oldest-first (Phase 12).
func (s *Server) ThreadComments(ctx context.Context, threadID string) ([]domain.Comment, error) {
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return nil, err
	}
	cl, inst, _, _, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return nil, err
	}
	// Base comments come from a short-lived cache (the expensive, paged Plane
	// fetch). It's viewer-agnostic — the per-caller "mine" flag and the live 👀
	// readers are applied to a copy below, so the cache is safe to share.
	base, err := s.commentsFor(ctx, cl, inst, wid)
	if err != nil {
		return nil, err
	}
	me, _ := domain.ActorFrom(ctx)
	out := make([]domain.Comment, len(base))
	copy(out, base)
	for i := range out {
		out[i].Mine = out[i].AuthorID != "" && out[i].AuthorID == me.ID
	}
	s.attachReaders(inst.Slug, out)
	return out, nil
}

// commentsFor returns a work item's rendered comments (oldest-first, without the
// viewer-specific "mine" flag or 👀 readers), served from a short-TTL cache.
// singleflight coalesces concurrent misses so a burst of opens hits Plane once.
func (s *Server) commentsFor(ctx context.Context, cl *plane.Client, inst domain.Instance, wid string) ([]domain.Comment, error) {
	s.commentsMu.Lock()
	if c, ok := s.commentsCache[wid]; ok && s.now().Before(c.exp) {
		s.commentsMu.Unlock()
		return c.comments, nil
	}
	s.commentsMu.Unlock()

	v, err, _ := s.sf.Do("comments:"+wid, func() (any, error) {
		cms, err := cl.ListComments(ctx, wid)
		if err != nil {
			return nil, err
		}
		names := s.memberNames(ctx, inst)
		out := make([]domain.Comment, 0, len(cms))
		for _, cm := range cms {
			out = append(out, renderComment(cm, names, "")) // viewer-agnostic (Mine=false)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
		s.commentsMu.Lock()
		s.commentsCache[wid] = cachedComments{comments: out, exp: s.now().Add(commentsTTL)}
		s.commentsMu.Unlock()
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]domain.Comment), nil
}

// invalidateComments drops a work item's cached comments (after a new post) so
// the next read reflects it immediately.
func (s *Server) invalidateComments(wid string) {
	s.commentsMu.Lock()
	delete(s.commentsCache, wid)
	s.commentsMu.Unlock()
}

// attachReaders decorates each comment with its 👀 read-receipts, excluding the
// comment's own author (you don't "read" your own message).
func (s *Server) attachReaders(instance string, cs []domain.Comment) {
	if len(cs) == 0 {
		return
	}
	ids := make([]string, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	byID, err := s.store.CommentReaders(instance, ids)
	if err != nil {
		return // read-receipts are best-effort, never fatal to reading comments
	}
	for i := range cs {
		for _, r := range byID[cs[i].ID] {
			if r.ID == cs[i].AuthorID {
				continue // author isn't a reader of their own comment
			}
			cs[i].Readers = append(cs[i].Readers, domain.Reader{ID: r.ID, Name: r.Name})
		}
	}
}

// MarkCommentsRead records the caller as having read the given comments (👀).
// Plane has no comment-reaction API, so this is a server overlay. Callers pass
// only comments they didn't author; the read display also excludes self-reads.
func (s *Server) MarkCommentsRead(ctx context.Context, threadID string, commentIDs []string) error {
	if len(commentIDs) == 0 {
		return nil
	}
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return err
	}
	_, inst, _, _, _, err := s.interiorClient(ctx, full) // also authorizes access
	if err != nil {
		return err
	}
	me, _ := domain.ActorFrom(ctx)
	if me.ID == "" {
		return fmt.Errorf("%w: no identity", errUnauth)
	}
	return s.store.MarkCommentsRead(inst.Slug, me.ID, me.Name, s.now().Format(time.RFC3339), commentIDs)
}

// renderComment normalizes a Plane comment through the same md pipeline used
// everywhere else (FromHTML → GFM → goldmark), resolving the author name and
// flagging the caller's own messages.
func renderComment(cm plane.Comment, names map[string]string, meID string) domain.Comment {
	author := names[cm.ActorID]
	if author == "" {
		author = "someone"
	}
	markdown := md.FromHTML(cm.HTML)
	return domain.Comment{
		ID:        cm.ID,
		Author:    author,
		AuthorID:  cm.ActorID,
		Markdown:  markdown,
		HTML:      md.RenderHTML(markdown),
		Mine:      cm.ActorID != "" && cm.ActorID == meID,
		CreatedAt: cm.CreatedAt,
	}
}

// PostComment adds a comment to a thread's discussion, written AS the caller so
// Plane attributes it correctly (impersonation via the presented Plane key, like
// birth). Comments are communication, not evidence — posting warms nothing (§4).
func (s *Server) PostComment(ctx context.Context, threadID, body string) (domain.Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.Comment{}, fmt.Errorf("%w: empty comment", errBadRequest)
	}
	full, err := s.resolveThreadID(ctx, threadID)
	if err != nil {
		return domain.Comment{}, err
	}
	_, inst, _, projID, wid, err := s.interiorClient(ctx, full)
	if err != nil {
		return domain.Comment{}, err
	}
	// Write with the caller's own key so Plane's native actor is the real author.
	wcl := plane.New(inst.BaseURL, s.writeKey(ctx, inst), inst.Workspace, projID)
	cm, err := wcl.CreateComment(ctx, wid, md.RenderHTML(body))
	if err != nil {
		return domain.Comment{}, err
	}
	s.invalidateComments(wid) // next read includes the new comment
	me, _ := domain.ActorFrom(ctx)
	names := s.memberNames(ctx, inst)
	out := renderComment(cm, names, me.ID)
	// The freshly-created comment is definitively ours, and Plane may not have
	// filled the author name into our members cache path — trust the actor.
	out.Mine = true
	if out.Author == "someone" && me.Name != "" {
		out.Author = me.Name
	}
	log.Printf("comment by %s on thread %s", me.Label(), wid)
	return out, nil
}

// ---- HTTP handlers ----

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.Timeline(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleThreadDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.ThreadDetail(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleThreadComments(w http.ResponseWriter, r *http.Request) {
	c, err := s.ThreadComments(r.Context(), r.PathValue("id"))
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handlePostComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body     string `json:"body"`
		Markdown string `json:"markdown"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	body := in.Body
	if body == "" {
		body = in.Markdown
	}
	cm, err := s.PostComment(r.Context(), r.PathValue("id"), body)
	if writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, cm)
}

func (s *Server) handleMarkCommentsRead(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CommentIDs []string `json:"comment_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if err := s.MarkCommentsRead(r.Context(), r.PathValue("id"), in.CommentIDs); writeErr(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
