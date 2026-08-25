package mcpapi

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"regexp"
	"sort"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AngelMaldonado/bubble-work/prompts"
)

// MCP prompts: the saga, and how to use the tools without fighting them.
//
// They exist because tool descriptions are the wrong place for a model. A tool
// description says what one call does; a prompt can say why the framework is shaped
// this way, which is what an agent needs before its first write. Both matter — the
// descriptions carry the contract, the prompts carry the reasoning.
//
// They live as MARKDOWN FILES under prompts/ (see that package for why), with a
// short frontmatter block for the metadata MCP needs:
//
//	---
//	name: work-a-thread
//	title: Work a thread
//	description: one line, shown in the client's prompt list
//	args: thread_id (required), type
//	---
//	the body, with {{thread_id}} substituted at call time
//
// A malformed file is logged and skipped rather than fatal: a typo in prose must not
// take down the tool surface. That lesson is already in this package's history —
// one bad tool definition once took every tool with it.

var frontmatterRe = regexp.MustCompile(`(?s)\A---\r?\n(.*?)\r?\n---\r?\n?`)

// prompt is one parsed markdown file.
type prompt struct {
	name, title, description, body string
	args                           []*sdk.PromptArgument
}

// parsePrompt reads the frontmatter and the body. Unknown keys are ignored so a file
// can carry notes for humans without breaking the loader.
func parsePrompt(filename, raw string) (prompt, error) {
	m := frontmatterRe.FindStringSubmatch(raw)
	if m == nil {
		return prompt{}, fmt.Errorf("%s: no --- frontmatter block", filename)
	}
	p := prompt{body: strings.TrimSpace(raw[len(m[0]):])}
	for _, line := range strings.Split(m[1], "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, val = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(val)
		switch key {
		case "name":
			p.name = val
		case "title":
			p.title = val
		case "description":
			p.description = val
		case "args":
			for _, a := range strings.Split(val, ",") {
				a = strings.TrimSpace(a)
				if a == "" {
					continue
				}
				required := strings.Contains(a, "(required)")
				a = strings.TrimSpace(strings.ReplaceAll(a, "(required)", ""))
				p.args = append(p.args, &sdk.PromptArgument{Name: a, Required: required})
			}
		}
	}
	if p.name == "" {
		return prompt{}, fmt.Errorf("%s: frontmatter has no name", filename)
	}
	if p.body == "" {
		return prompt{}, fmt.Errorf("%s: no body after the frontmatter", filename)
	}
	return p, nil
}

// render substitutes {{arg}} placeholders and resolves {{#arg}}…{{/arg}} blocks,
// which are kept only when the argument was supplied.
//
// Two constructs, chosen because a prompt that needs a third is a prompt trying to be
// a program. An argument nobody passed renders as nothing rather than as the literal
// {{name}}: a stray placeholder in an agent's context is worse than a slightly
// shorter sentence.
func (p prompt) render(args map[string]string) string {
	out := p.body
	for _, a := range p.args {
		v := strings.TrimSpace(args[a.Name])
		open, close := "{{#"+a.Name+"}}", "{{/"+a.Name+"}}"
		if v == "" {
			out = dropBlock(out, open, close)
		} else {
			out = strings.ReplaceAll(strings.ReplaceAll(out, open, ""), close, "")
		}
		out = strings.ReplaceAll(out, "{{"+a.Name+"}}", v)
	}
	// Anything left is a placeholder the frontmatter never declared.
	return strings.TrimSpace(unknownPlaceholderRe.ReplaceAllString(out, ""))
}

var unknownPlaceholderRe = regexp.MustCompile(`\{\{[#/]?[a-zA-Z0-9_]+\}\}`)

func dropBlock(s, open, close string) string {
	for {
		i := strings.Index(s, open)
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], close)
		if j < 0 {
			return strings.Replace(s, open, "", 1)
		}
		s = s[:i] + s[i+j+len(close):]
	}
}

// loadPrompts parses every embedded markdown file, sorted by name so registration
// order is stable.
func loadPrompts(fsys fs.FS) []prompt {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		log.Printf("mcp prompts: reading the embedded set: %v", err)
		return nil
	}
	var out []prompt
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			log.Printf("mcp prompts: %s: %v", e.Name(), err)
			continue
		}
		p, err := parsePrompt(e.Name(), string(raw))
		if err != nil {
			log.Printf("mcp prompts: skipped — %v", err)
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// addPrompts registers the embedded prompts on an MCP server.
func addPrompts(srv *sdk.Server) int {
	ps := loadPrompts(prompts.FS())
	for _, p := range ps {
		p := p
		srv.AddPrompt(&sdk.Prompt{
			Name: p.name, Title: p.title, Description: p.description, Arguments: p.args,
		}, func(ctx context.Context, req *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			var args map[string]string
			if req != nil && req.Params != nil {
				args = req.Params.Arguments
			}
			return &sdk.GetPromptResult{
				Description: p.description,
				Messages: []*sdk.PromptMessage{{
					Role:    "user",
					Content: &sdk.TextContent{Text: p.render(args)},
				}},
			}, nil
		})
	}
	return len(ps)
}
