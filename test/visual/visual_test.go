package visual

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/orisano/pixelmatch"
)

const (
	baselineDir       = "baseline"
	stampSourceName   = "letter-with-logo"
	stampText         = "7000-0000-0001"
	diffThreshold     = 0.1
	renderDPI         = "100"
	healthWaitTimeout = 60 * time.Second
)

var (
	gotenbergURL    = envOr("GOTENBERG_URL", "http://localhost:3000")
	resultsDir      = envOr("RESULTS_DIR", "test-results/images")
	updateBaselines = os.Getenv("UPDATE_BASELINES") != ""
	httpClient      = &http.Client{Timeout: 60 * time.Second}
)

type formPart struct {
	name     string
	filename string
	content  []byte
}

func TestMain(m *testing.M) {
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "creating results directory: %v\n", err)
		os.Exit(1)
	}

	if err := waitForHealthy(healthWaitTimeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestConvertHTML(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join(baselineDir, "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatalf("no HTML fixtures found in %s", baselineDir)
	}

	for _, fixture := range fixtures {
		name := strings.TrimSuffix(filepath.Base(fixture), ".html")

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pdf := convertHTML(t, name, readFile(t, fixture))
			pdfPath := writeResult(t, name+".pdf", pdf)
			pages := pageCount(t, pdfPath)

			if got := siriusPageCount(pdf); got != pages {
				t.Errorf("Sirius would count %d page(s) but pdfinfo reports %d", got, pages)
			}

			compareWithBaseline(t, name, pdfPath, pages)
		})
	}
}

func TestStamp(t *testing.T) {
	source := filepath.Join(baselineDir, stampSourceName+".html")
	pdf := convertHTML(t, stampSourceName, readFile(t, source))

	for _, offsetY := range []int{20, 60} {
		name := fmt.Sprintf("stamp-offset-%d", offsetY)

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stamped := stampPDF(t, pdf, stampText, offsetY)
			pdfPath := writeResult(t, name+".pdf", stamped)

			compareWithBaseline(t, name, pdfPath, pageCount(t, pdfPath))
		})
	}
}

func convertHTML(t *testing.T, title string, html []byte) []byte {
	t.Helper()

	return postForm(t, "/forms/chromium/convert/html", []formPart{
		{name: "files", filename: "index.html", content: html},
		{name: "paperWidth", content: []byte("8.27in")},
		{name: "paperHeight", content: []byte("11.7in")},
		{name: "marginTop", content: []byte("15px")},
		{name: "marginBottom", content: []byte("70px")},
		{name: "marginLeft", content: []byte("70px")},
		{name: "marginRight", content: []byte("70px")},
		{name: "title", content: []byte(title)},
	})
}

func stampPDF(t *testing.T, pdf []byte, text string, offsetY int) []byte {
	t.Helper()

	options := fmt.Sprintf(`{"rotation": "0", "scalefactor": "0.3 abs", "pos": "br", "off": "-75 %d"}`, offsetY)

	return postForm(t, "/forms/pdfengines/stamp", []formPart{
		{name: "files", filename: "file.pdf", content: pdf},
		{name: "stampSource", content: []byte("text")},
		{name: "stampExpression", content: []byte(text)},
		{name: "stampOptions", content: []byte(options)},
	})
}

func postForm(t *testing.T, path string, parts []formPart) []byte {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	for _, p := range parts {
		var (
			part io.Writer
			err  error
		)

		if p.filename != "" {
			header := textproto.MIMEHeader{}
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, p.name, p.filename))
			header.Set("Content-Type", contentTypeFor(p.filename))
			part, err = writer.CreatePart(header)
		} else {
			part, err = writer.CreateFormField(p.name)
		}
		if err != nil {
			t.Fatalf("creating form part %s: %v", p.name, err)
		}

		if _, err := part.Write(p.content); err != nil {
			t.Fatalf("writing form part %s: %v", p.name, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	resp, err := httpClient.Post(gotenbergURL+path, writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response from %s: %v", path, err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s returned %d: %s", path, resp.StatusCode, truncate(data, 500))
	}

	if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/pdf") {
		t.Fatalf("POST %s returned content type %q, expected application/pdf", path, contentType)
	}

	return data
}

func compareWithBaseline(t *testing.T, name, pdfPath string, pages int) {
	t.Helper()

	generated := make([]string, pages)
	for page := 1; page <= pages; page++ {
		generated[page-1] = rasterise(t, pdfPath, page, filepath.Join(resultsDir, fmt.Sprintf("%s.%d", name, page)))
	}

	if updateBaselines {
		writeBaselines(t, name, generated)
		return
	}

	baselines := baselinePages(name)
	if len(baselines) == 0 {
		t.Fatalf("no baseline images for %s; run make update-baselines", name)
	}

	if len(baselines) != pages {
		t.Errorf("page count changed: baseline has %d page(s), generated PDF has %d", len(baselines), pages)
	}

	for i := 0; i < min(len(baselines), pages); i++ {
		comparePage(t, baselines[i], generated[i], filepath.Join(resultsDir, fmt.Sprintf("%s.%d.diff.png", name, i+1)))
	}
}

func comparePage(t *testing.T, baselinePath, generatedPath, diffPath string) {
	t.Helper()

	baseline := readPNG(t, baselinePath)
	generated := readPNG(t, generatedPath)

	if baseline.Bounds().Size() != generated.Bounds().Size() {
		t.Errorf("%s: size changed from %v to %v", filepath.Base(generatedPath), baseline.Bounds().Size(), generated.Bounds().Size())
		return
	}

	var diff image.Image
	count, err := pixelmatch.MatchPixel(baseline, generated, pixelmatch.Threshold(diffThreshold), pixelmatch.WriteTo(&diff))
	if err != nil {
		t.Fatalf("comparing %s: %v", filepath.Base(generatedPath), err)
	}

	if count == 0 {
		return
	}

	writePNG(t, diffPath, diff)
	t.Errorf("%s: %d pixel(s) differ from baseline; diff written to %s", filepath.Base(generatedPath), count, diffPath)
}

func rasterise(t *testing.T, pdfPath string, page int, outPrefix string) string {
	t.Helper()

	pageArg := strconv.Itoa(page)
	cmd := exec.Command("pdftoppm", "-png", "-r", renderDPI, "-f", pageArg, "-l", pageArg, "-singlefile", pdfPath, outPrefix)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pdftoppm page %d of %s: %v: %s", page, pdfPath, err, out)
	}

	return outPrefix + ".png"
}

func pageCount(t *testing.T, pdfPath string) int {
	t.Helper()

	out, err := exec.Command("pdfinfo", pdfPath).Output()
	if err != nil {
		t.Fatalf("pdfinfo %s: %v", pdfPath, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		field, value, found := strings.Cut(scanner.Text(), ":")
		if !found || strings.TrimSpace(field) != "Pages" {
			continue
		}

		pages, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			t.Fatalf("parsing page count %q: %v", value, err)
		}

		return pages
	}

	t.Fatalf("pdfinfo output for %s has no page count", pdfPath)
	return 0
}

func siriusPageCount(pdf []byte) int {
	count := bytes.Count(pdf, []byte("/Type /Page ")) + bytes.Count(pdf, []byte("/Type /Page\n"))
	if count == 0 {
		return 1
	}

	return count
}

func baselinePages(name string) []string {
	var pages []string

	for page := 1; ; page++ {
		path := baselinePath(name, page)
		if _, err := os.Stat(path); err != nil {
			return pages
		}
		pages = append(pages, path)
	}
}

func writeBaselines(t *testing.T, name string, generated []string) {
	t.Helper()

	for _, path := range baselinePages(name) {
		if err := os.Remove(path); err != nil {
			t.Fatalf("removing old baseline %s: %v", path, err)
		}
	}

	for i, source := range generated {
		target := baselinePath(name, i+1)
		if err := os.WriteFile(target, readFile(t, source), 0o644); err != nil {
			t.Fatalf("writing baseline %s: %v", target, err)
		}
	}

	t.Logf("updated %d baseline page(s) for %s", len(generated), name)
}

func baselinePath(name string, page int) string {
	return filepath.Join(baselineDir, fmt.Sprintf("%s.%d.png", name, page))
}

func waitForHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	var lastErr error
	for {
		resp, err := httpClient.Get(gotenbergURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		lastErr = err

		if time.Now().After(deadline) {
			return fmt.Errorf("gotenberg at %s not healthy after %s: %v", gotenbergURL, timeout, lastErr)
		}

		time.Sleep(time.Second)
	}
}

func readPNG(t *testing.T, path string) image.Image {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decoding %s: %v", path, err)
	}

	return img
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encoding %s: %v", path, err)
	}
}

func writeResult(t *testing.T, filename string, data []byte) string {
	t.Helper()

	path := filepath.Join(resultsDir, filename)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}

	return path
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	return data
}

func contentTypeFor(filename string) string {
	if contentType := mime.TypeByExtension(filepath.Ext(filename)); contentType != "" {
		return contentType
	}

	return "application/octet-stream"
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func truncate(data []byte, limit int) string {
	if len(data) <= limit {
		return string(data)
	}

	return string(data[:limit]) + "…"
}
