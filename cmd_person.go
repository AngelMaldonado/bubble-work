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
			fresh := err != nil
			if fresh {
				r = core.NewRecord(col)
				r.Set("email", email)
			}

			// The password is written ONLY when it is actually different.
			//
			// `SetPassword` rotates the record's `tokenKey`, and that key is part
			// of what signs its auth tokens — so re-applying the same password
			// invalidates every token that person holds. `just dev` runs this on
			// every start so a fresh clone can sign in, which meant every restart
			// silently signed the operator out of the browser AND killed the token
			// their agent was using. Nothing had changed; the write itself was the
			// damage.
			changed := fresh
			if fresh || !r.ValidatePassword(password) {
				r.SetPassword(password)
				changed = true
			}
			if r.GetString("role") != role {
				r.Set("role", role)
				changed = true
			}
			// Verified, because this is somebody an operator is vouching for from
			// the command line; an unverified account cannot sign in.
			if !r.Verified() {
				r.SetVerified(true)
				changed = true
			}
			if !changed {
				fmt.Printf("person %q already as asked (role: %s)\n", email, role)
				return nil
			}
			if err := app.Save(r); err != nil {
				return err
			}
			fmt.Printf("person %q saved (role: %s)\n", email, role)
			return nil
		},
	}
}
