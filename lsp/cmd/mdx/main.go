package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/olegmif/mdx/lsp/internal/cli"
	"github.com/olegmif/mdx/lsp/internal/db"
)

var (
	flagDB       string
	flagExcludes []string
	flagQuiet    bool
)

var rootCmd = &cobra.Command{
	Use:   "mdx",
	Short: "mdx - markdown notes indexer and LSP server",
}

var scanCmd = &cobra.Command{
	Use:   "scan [path...]",
	Short: "Scan filesystem for .md files and update the index",
	RunE:  runScan,
}

func init() {
	scanCmd.Flags().StringVar(&flagDB, "db", "",
		"path to SQLite database (default: $XDG_DATA_HOME/mdx/mdx.db)")
	scanCmd.Flags().StringSliceVar(&flagExcludes, "exclude", nil,
		"extra directory names to skip (added to defaults)")
	scanCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false,
		"suppress summary line")
	rootCmd.AddCommand(scanCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runScan(cmd *cobra.Command, args []string) error {
	roots := args
	if len(roots) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve $HOME: %w", err)
		}
		roots = []string{home}
	}

	dbPath, err := db.ResolvePath(flagDB)
	if err != nil {
		return err
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := db.Migrate(conn); err != nil {
		return err
	}

	excludes := append([]string{}, cli.DefaultExcludes...)
	excludes = append(excludes, flagExcludes...)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	stats, err := cli.Run(ctx, conn, roots, excludes)
	if err != nil {
		return err
	}

	if !flagQuiet {
		fmt.Printf("scanned: %d, errors: %d, elapsed: %s\n",
			stats.Files, stats.Errors, stats.Elapsed)
	}
	return nil
}
