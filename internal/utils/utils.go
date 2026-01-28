package utils

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func ExtractFileInfo(url string) (name string, sizeStr string) {
	slog.Info("Extracting file info", "url", url)
	res, err := http.Get(url)
	if err != nil {
		slog.Error("Failed to fetch URL", "error", err)
		return "Unknown", "Unknown size"
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		slog.Warn("Non-200 status code", "status", res.StatusCode)
		return "Unknown", "Unknown size"
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		slog.Error("Failed to parse HTML", "error", err)
		return "Unknown", "Unknown size"
	}

	title := "Unknown"
	doc.Find(".section_title").Each(func(i int, s *goquery.Selection) {
		if t, ok := s.Attr("title"); ok {
			title = t
		}
	})

	size := "Unknown size"
	doc.Find(".footer-video-size").Each(func(i int, s *goquery.Selection) {
		size = strings.TrimSpace(s.Text())
	})

	return title, size
}

func ParseSize(sizeStr string) int64 {
	sizeStr = strings.ReplaceAll(sizeStr, "&nbsp;", " ")
	parts := strings.Fields(sizeStr)
	if len(parts) != 2 {
		return 0
	}
	value, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToUpper(parts[1])
	var multiplier float64
	switch unit {
	case "B":
		multiplier = 1
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0
	}
	return int64(value * multiplier)
}
