package cmd

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/store"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "migrate", Short: "Manage database schema migrations"}
	run := func(fn func(cmd *cobra.Command, st *store.Store) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			st, err := openStore(cmd.Context(), cfg, false)
			if err != nil {
				return err
			}
			defer st.DB().Close()
			return fn(cmd, st)
		}
	}
	cmd.AddCommand(&cobra.Command{Use: "up", Short: "Apply all pending migrations", RunE: run(func(cmd *cobra.Command, st *store.Store) error {
		n, err := st.DB().MigrateUp(cmd.Context())
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "applied %d migration(s)\n", n)
		return nil
	})})
	cmd.AddCommand(&cobra.Command{Use: "down", Short: "Roll back the most recent migration", RunE: run(func(cmd *cobra.Command, st *store.Store) error {
		m, err := st.DB().MigrateDown(cmd.Context())
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "rolled back %04d_%s\n", m.Version, m.Name)
		return nil
	})})
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "List migrations and their state", RunE: run(func(cmd *cobra.Command, st *store.Store) error {
		list, err := st.DB().Status(cmd.Context())
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "VERSION\tNAME\tSTATE")
		for _, m := range list {
			state := "pending"
			if m.Applied {
				state = "applied"
			}
			fmt.Fprintf(w, "%04d\t%s\t%s\n", m.Version, m.Name, state)
		}
		return w.Flush()
	})})
	return cmd
}

func readPassword(cmd *cobra.Command, prompt string) (string, error) {
	if f, ok := cmd.InOrStdin().(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(cmd.ErrOrStderr(), prompt)
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		return string(b), err
	}
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no password provided on stdin")
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Manage users directly in the database"}
	withStore := func(fn func(cmd *cobra.Command, st *store.Store, args []string) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			st, err := openStore(cmd.Context(), cfg, true)
			if err != nil {
				return err
			}
			defer st.DB().Close()
			return fn(cmd, st, args)
		}
	}

	var password string
	var admin, mustChange bool
	create := &cobra.Command{Use: "create <username>", Short: "Create a user", Args: cobra.ExactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, st *store.Store, args []string) error {
			pw := password
			if pw == "" {
				var err error
				if pw, err = readPassword(cmd, "Password: "); err != nil {
					return err
				}
			}
			hash, err := auth.HashPassword(pw)
			if err != nil {
				return err
			}
			u := &store.User{Username: args[0], PasswordHash: hash, PasswordScheme: store.SchemeBcrypt, MustChangePassword: mustChange}
			if err := st.CreateUser(cmd.Context(), u); err != nil {
				if errors.Is(err, store.ErrConflict) {
					return fmt.Errorf("user %q already exists", args[0])
				}
				return err
			}
			if admin {
				if err := st.GrantSystem(cmd.Context(), store.SubjectUser, u.ID, store.PermAdminister); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created user %s (id %d)\n", u.Username, u.ID)
			return nil
		})}
	create.Flags().StringVar(&password, "password", "", "password (read from stdin when omitted)")
	create.Flags().BoolVar(&admin, "admin", false, "grant the ADMINISTER system permission")
	create.Flags().BoolVar(&mustChange, "must-change", false, "require a password change at first login")

	passwd := &cobra.Command{Use: "passwd <username>", Short: "Set a user's password and end its sessions", Args: cobra.ExactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, st *store.Store, args []string) error {
			u, err := st.GetUserByName(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("user %q not found", args[0])
			}
			pw, err := readPassword(cmd, "New password: ")
			if err != nil {
				return err
			}
			hash, err := auth.HashPassword(pw)
			if err != nil {
				return err
			}
			if err := st.SetPassword(cmd.Context(), u.ID, hash, store.SchemeBcrypt, "", false); err != nil {
				return err
			}
			_, _ = st.DeleteUserSessions(cmd.Context(), u.ID, "")
			fmt.Fprintf(cmd.OutOrStdout(), "password updated for %s\n", u.Username)
			return nil
		})}

	list := &cobra.Command{Use: "list", Short: "List users",
		RunE: withStore(func(cmd *cobra.Command, st *store.Store, _ []string) error {
			users, _, err := st.ListUsers(cmd.Context(), "", store.Page{Limit: 500})
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tUSERNAME\tDISABLED\tMUST_CHANGE\tCREATED")
			for _, u := range users {
				fmt.Fprintf(w, "%d\t%s\t%t\t%t\t%s\n", u.ID, u.Username, u.Disabled, u.MustChangePassword, time.UnixMilli(u.CreatedAt).Format(time.DateTime))
			}
			return w.Flush()
		})}

	disable := &cobra.Command{Use: "disable <username>", Short: "Disable a user and end its sessions", Args: cobra.ExactArgs(1),
		RunE: withStore(func(cmd *cobra.Command, st *store.Store, args []string) error {
			u, err := st.GetUserByName(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("user %q not found", args[0])
			}
			if err := st.SetUserDisabled(cmd.Context(), u.ID, true); err != nil {
				return err
			}
			_, _ = st.DeleteUserSessions(cmd.Context(), u.ID, "")
			fmt.Fprintf(cmd.OutOrStdout(), "disabled %s\n", u.Username)
			return nil
		})}
	cmd.AddCommand(create, passwd, list, disable)
	return cmd
}

func newHealthcheckCmd() *cobra.Command {
	var url string
	var insecure bool
	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Exit 0 when the server's /health endpoint answers 200",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if url == "" {
				scheme, addr := "http", ":8080"
				if v := os.Getenv("APP_ADDR"); v != "" {
					addr = v
				}
				if strings.EqualFold(os.Getenv("TLS_ENABLED"), "true") {
					scheme, insecure = "https", true
				}
				if strings.HasPrefix(addr, ":") {
					addr = "127.0.0.1" + addr
				}
				url = scheme + "://" + addr + "/health"
			}
			client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}}} //nolint:gosec // local probe
			resp, err := client.Get(url)
			if err != nil {
				return err
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health returned %d", resp.StatusCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "health URL (default derived from APP_ADDR/TLS_ENABLED)")
	cmd.Flags().BoolVar(&insecure, "insecure", false, "skip TLS certificate verification")
	return cmd
}
