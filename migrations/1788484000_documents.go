package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Phase 1b: where a workspace's markdown lives, and where a thread's document
// sits inside it.
//
// Neither column is written by a client. `repo_path` is stamped when a workspace
// is founded and never changes — the directory is a git repository, and renaming
// a workspace must not orphan its history. `doc_path` is stamped when a thread is
// created and FOLLOWS a rename, because a tree whose filenames disagree with the
// threads they hold is a tree nobody trusts.
func init() {
	m.Register(func(app core.App) error {
		ws, err := app.FindCollectionByNameOrId("workspaces")
		if err != nil {
			return err
		}
		ws.Fields.Add(&core.TextField{Name: "repo_path", Max: 200})
		if err := app.Save(ws); err != nil {
			return err
		}

		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		threads.Fields.Add(&core.TextField{Name: "doc_path", Max: 400})
		return app.Save(threads)
	}, func(app core.App) error {
		ws, err := app.FindCollectionByNameOrId("workspaces")
		if err == nil {
			ws.Fields.RemoveByName("repo_path")
			if err := app.Save(ws); err != nil {
				return err
			}
		}
		threads, err := app.FindCollectionByNameOrId("threads")
		if err != nil {
			return err
		}
		threads.Fields.RemoveByName("doc_path")
		return app.Save(threads)
	})
}
