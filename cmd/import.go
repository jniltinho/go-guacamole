package cmd

import (
	"database/sql"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jniltinho/go-guacamole/internal/importer"
)

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "import", Short: "Import data from other systems"}
	cmd.AddCommand(newImportGuacamoleCmd())
	return cmd
}

func newImportGuacamoleCmd() *cobra.Command {
	var srcDriver, dsn, conflict string
	var dry, accept bool
	cmd := &cobra.Command{
		Use:   "guacamole",
		Short: "Import users, groups, connections and permissions from an Apache Guacamole database (read-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dsn == "" {
				return fmt.Errorf("--dsn is required")
			}
			if conflict != "skip" && conflict != "update" {
				return fmt.Errorf("--conflict must be skip or update")
			}
			driver := map[string]string{"mysql": "mysql", "postgres": "pgx"}[srcDriver]
			if driver == "" {
				return fmt.Errorf("--source-driver must be mysql or postgres")
			}
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			dst, err := openStore(cmd.Context(), cfg, true)
			if err != nil {
				return err
			}
			defer dst.DB().Close()
			src, err := sql.Open(driver, dsn)
			if err != nil {
				return err
			}
			defer src.Close()
			if err := src.PingContext(cmd.Context()); err != nil {
				return fmt.Errorf("connect to source: %w", err)
			}
			rep, err := importer.Run(cmd.Context(), src, dst, importer.Options{SourceDriver: srcDriver, DryRun: dry,
				Update: conflict == "update", AcceptExpansion: accept})
			if rep != nil {
				rep.Print(cmd.OutOrStdout(), dry)
			}
			if err != nil {
				return err
			}
			if !dry {
				_ = dst.BumpAccessEpoch(cmd.Context())
				// recording parameters of imported connections become recording policy overrides
				items, err := dst.ApplyRecordingInventory(cmd.Context())
				for _, it := range items {
					fmt.Fprintf(cmd.OutOrStdout(), "RECORDING  %s: video=%v text=%v keys=%v (removed %v)\n",
						it.ConnectionName, it.Video, it.Text, it.Keys, it.Removed)
				}
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&srcDriver, "source-driver", "mysql", "source database driver: mysql or postgres")
	cmd.Flags().StringVar(&dsn, "dsn", "", "source Guacamole database DSN")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "show what would be imported without writing")
	cmd.Flags().StringVar(&conflict, "conflict", "skip", "existing objects: skip or update")
	cmd.Flags().BoolVar(&accept, "accept-expansion", false, "convert balancing-group READ into READ on each child connection")
	return cmd
}
