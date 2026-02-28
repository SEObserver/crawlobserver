package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/SEObserver/crawlobserver/internal/storage"
)

// WriteExternalLinks writes an external links report.
func WriteExternalLinks(w io.Writer, links []storage.LinkRow, format string) error {
	switch format {
	case "csv":
		return writeCSV(w, links)
	case "table":
		return writeTable(w, links)
	default:
		return writeTable(w, links)
	}
}

func writeCSV(w io.Writer, links []storage.LinkRow) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	if err := writer.Write([]string{"source_url", "target_url", "anchor_text", "rel", "tag"}); err != nil {
		return err
	}

	for _, l := range links {
		if err := writer.Write([]string{l.SourceURL, l.TargetURL, l.AnchorText, l.Rel, l.Tag}); err != nil {
			return err
		}
	}
	return nil
}

func writeTable(w io.Writer, links []storage.LinkRow) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SOURCE\tTARGET\tANCHOR\tREL\tTAG")
	fmt.Fprintln(tw, "------\t------\t------\t---\t---")

	for _, l := range links {
		anchor := l.AnchorText
		if len(anchor) > 60 {
			anchor = anchor[:57] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			truncate(l.SourceURL, 80),
			truncate(l.TargetURL, 80),
			anchor,
			l.Rel,
			l.Tag,
		)
	}
	tw.Flush()
	fmt.Fprintf(w, "\nTotal: %d external links\n", len(links))
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
