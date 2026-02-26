package cli

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/SEObserver/seocrawler/internal/config"
	"github.com/SEObserver/seocrawler/internal/crawler"
	"github.com/SEObserver/seocrawler/internal/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var crawlCmd = &cobra.Command{
	Use:   "crawl",
	Short: "Start a crawl session",
	Long:  `Crawl one or more seed URLs, extracting SEO signals and storing results in ClickHouse.`,
	RunE:  runCrawl,
}

func init() {
	rootCmd.AddCommand(crawlCmd)

	crawlCmd.Flags().String("seed", "", "Seed URL to crawl")
	crawlCmd.Flags().String("seeds-file", "", "File with seed URLs (one per line, optional tab-separated priority)")
	crawlCmd.Flags().Duration("delay", 0, "Delay between requests to the same host")
	crawlCmd.Flags().Int("max-pages", 0, "Maximum number of pages to crawl (0 = unlimited)")
	crawlCmd.Flags().Int("max-depth", 0, "Maximum crawl depth (0 = unlimited)")
	crawlCmd.Flags().Int("workers", 0, "Number of concurrent fetch workers")
	crawlCmd.Flags().Bool("store-html", false, "Store raw HTML body (ZSTD compressed in ClickHouse)")

	viper.BindPFlag("crawler.delay", crawlCmd.Flags().Lookup("delay"))
	viper.BindPFlag("crawler.max_pages", crawlCmd.Flags().Lookup("max-pages"))
	viper.BindPFlag("crawler.max_depth", crawlCmd.Flags().Lookup("max-depth"))
	viper.BindPFlag("crawler.workers", crawlCmd.Flags().Lookup("workers"))
	viper.BindPFlag("crawler.store_html", crawlCmd.Flags().Lookup("store-html"))
}

func runCrawl(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Collect seed URLs
	var seeds []string

	seed, _ := cmd.Flags().GetString("seed")
	if seed != "" {
		seeds = append(seeds, seed)
	}

	seedsFile, _ := cmd.Flags().GetString("seeds-file")
	if seedsFile != "" {
		fileSeeds, err := readSeedsFile(seedsFile)
		if err != nil {
			return fmt.Errorf("reading seeds file: %w", err)
		}
		seeds = append(seeds, fileSeeds...)
	}

	if len(seeds) == 0 {
		return fmt.Errorf("no seed URLs provided. Use --seed or --seeds-file")
	}

	// Connect to ClickHouse
	store, err := storage.NewStore(
		cfg.ClickHouse.Host,
		cfg.ClickHouse.Port,
		cfg.ClickHouse.Database,
		cfg.ClickHouse.Username,
		cfg.ClickHouse.Password,
	)
	if err != nil {
		return fmt.Errorf("connecting to ClickHouse: %w", err)
	}
	defer store.Close()

	// Create engine
	engine := crawler.NewEngine(cfg, store)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Received shutdown signal, stopping gracefully...")
		engine.Stop()
	}()

	return engine.Run(seeds)
}

func readSeedsFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var seeds []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Support tab-separated format: URL\tpriority
		parts := strings.SplitN(line, "\t", 2)
		url := strings.TrimSpace(parts[0])
		if url != "" {
			seeds = append(seeds, url)
		}
	}
	return seeds, scanner.Err()
}
