package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/olegmif/mdx/lsp/internal/cli"
	"github.com/olegmif/mdx/lsp/internal/config"
	"github.com/olegmif/mdx/lsp/internal/db"
	"github.com/olegmif/mdx/lsp/internal/lsp"
)

var (
	flagDB          string
	flagIgnore      string
	flagExcludes    []string
	flagQuiet       bool
	flagLog         string
	flagEmbedConfig string
	flagEmbedModel  string
	flagEmbedAll    bool
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

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Remove notes rows for files that no longer exist or fall under ignore",
	Args:  cobra.NoArgs,
	RunE:  runGC,
}

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Run the LSP server on stdio",
	RunE:  runLSP,
}

var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Compute embeddings for indexed notes and upsert them into Qdrant",
	Args:  cobra.NoArgs,
	RunE:  runEmbed,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagDB, "db", "", "path to SQLite database (default: $XDG_DATA_HOME/mdx/mdx.db)")
	lspCmd.Flags().StringVar(&flagLog, "log", "", "path to LSP log file (default: $XDG_STATE_HOME/mdx/lsp.log)")
	lspCmd.Flags().StringVar(&flagIgnore, "ignore", "", "path to ignore file (default: $XDG_CONFIG_HOME/mdx/ignore)")

	scanCmd.Flags().StringVar(&flagIgnore, "ignore", "", "path to ignore file (default: $XDG_CONFIG_HOME/mdx/ignore)")
	scanCmd.Flags().StringSliceVar(&flagExcludes, "exclude", nil,
		"extra directory names to skip (added to defaults)")
	scanCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "suppress summary line")

	gcCmd.Flags().StringVar(&flagIgnore, "ignore", "", "path to ignore file (default: $XDG_CONFIG_HOME/mdx/ignore)")
	gcCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "suppress summary line")

	embedCmd.Flags().StringVar(&flagEmbedConfig, "embedding-config", "",
		"path to embedding config (default: $XDG_CONFIG_HOME/mdx/embedding.yaml)")
	embedCmd.Flags().StringVar(&flagEmbedModel, "model", "",
		"limit run to a single model name from config")
	embedCmd.Flags().BoolVar(&flagEmbedAll, "all", false,
		"ignore embeddings table and recompute every note")
	embedCmd.Flags().BoolVarP(&flagQuiet, "quiet", "q", false, "suppress summary line")

	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(gcCmd)
	rootCmd.AddCommand(lspCmd)
	rootCmd.AddCommand(embedCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// resolveRoots returns the user-provided roots, falling back to $HOME if none.
func resolveRoots(args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve $HOME: %w", err)
	}
	return []string{home}, nil
}

// loadIgnoreConfig resolves the ignore-file path, reads it and returns
// absolute prefixes. Read errors and per-line warnings are reported to
// stderr; the function continues with whatever prefixes were parsed.
func loadIgnoreConfig(flag string) ([]string, error) {
	ignorePath, err := config.ResolveIgnorePath(flag)
	if err != nil {
		return nil, err
	}
	prefixes, warnings, err := config.LoadIgnore(ignorePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdx: ignore: %v (continuing without ignore list)\n", err)
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "mdx: ignore: %s\n", w)
	}
	return prefixes, nil
}

func runScan(cmd *cobra.Command, args []string) error {
	roots, err := resolveRoots(args)
	if err != nil {
		return err
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

	ignorePrefixes, err := loadIgnoreConfig(flagIgnore)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	stats, err := cli.RunScan(ctx, conn, roots, excludes, ignorePrefixes)
	if err != nil {
		return err
	}

	if !flagQuiet {
		fmt.Printf("scanned: %d, errors: %d, elapsed: %s\n",
			stats.Files, stats.Errors, stats.Elapsed)
	}
	return nil
}

func runGC(cmd *cobra.Command, args []string) error {
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

	ignorePrefixes, err := loadIgnoreConfig(flagIgnore)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	stats, err := cli.RunGC(ctx, conn, ignorePrefixes)
	if err != nil {
		return err
	}

	if !flagQuiet {
		fmt.Printf("removed: %d, kept: %d, elapsed: %s\n",
			stats.Deleted, stats.Kept, stats.Elapsed)
	}
	return nil
}

func runLSP(cmd *cobra.Command, args []string) error {
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

	logPath, err := lsp.ResolveLogPath(flagLog)
	if err != nil {
		return err
	}

	ignorePrefixes, err := loadIgnoreConfig(flagIgnore)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return cli.RunLSP(ctx, conn, dbPath, logPath, ignorePrefixes)
}

func runEmbed(cmd *cobra.Command, args []string) error {
	cfgPath, err := config.ResolveEmbeddingPath(flagEmbedConfig)
	if err != nil {
		return err
	}
	cfg, warnings, err := config.LoadEmbedding(cfgPath)
	if err != nil {
		return fmt.Errorf("embedding config (%s): %w", cfgPath, err)
	}
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "mdx: embedding: %s\n", w)
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	_, err = cli.RunEmbed(ctx, conn, cfg, cli.EmbedOptions{
		Model: flagEmbedModel,
		All:   flagEmbedAll,
	})
	return err
}
