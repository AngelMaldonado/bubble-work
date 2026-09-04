package main

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// The `person` command: create or update somebody who WORKS here.
//
// PocketBase ships `superuser` for the account that operates the box, and there
// was no equivalent for the account that does the work — so the only way to make
// the first person was the dashboard, and a fresh clone could reach the dashboard
// while being unable to sign in to the UI at all.
//
// The same human usually wants both, with the same email. They are two records in
// two collections; PocketBase keeps them apart and they do not collide.
func personCommand(app *pocketbase.PocketBase) *cobra.Command {
	return &cobra.Command{
		Use:          "person <email> <password> [lead|member]",
		Short:        "Create or update a person (the account that works)",
		Args:         cobra.RangeArgs(2, 3),
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			email, password := args[0], args[1]
			role := "member"
			if len(args) == 3 {
				role = args[2]
			}
			if role != "lead" && role != "member" {
				return fmt.Errorf("role must be lead or member")
			}

			col, err := app.FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}
			r, err := app.FindAuthRecordByEmail("users", email)
			if err != nil {
				r = core.NewRecord(col)
				r.Set("email", email)
			}
			r.SetPassword(password)
			r.Set("role", role)
			// Verified, because this is somebody an operator is vouching for from
			// the command line; an unverified account cannot sign in.
			r.SetVerified(true)
			if err := app.Save(r); err != nil {
				return err
			}
			fmt.Printf("person %q saved (role: %s)\n", email, role)
			return nil
		},
	}
}
