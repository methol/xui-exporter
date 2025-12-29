package parse

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// ParsedSubscription represents the raw data extracted from the subscription HTML
type ParsedSubscription struct {
	SID          string
	DownloadByte int64
	UploadByte   int64
	TotalByte    int64
	Expire       int64
}

// ParseSubscription parses the subscription HTML and extracts data from
// template#subscription-data element's data-* attributes.
// Returns ParsedSubscription on success, or error if parsing/validation fails.
func ParseSubscription(htmlBytes []byte) (ParsedSubscription, error) {
	doc, err := html.Parse(strings.NewReader(string(htmlBytes)))
	if err != nil {
		return ParsedSubscription{}, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Find template#subscription-data element
	templateNode := findTemplateNode(doc)
	if templateNode == nil {
		return ParsedSubscription{}, fmt.Errorf("template#subscription-data not found")
	}

	// Extract attributes
	attrs := extractAttributes(templateNode)

	// Parse required fields
	sid, ok := attrs["data-sid"]
	if !ok || sid == "" {
		return ParsedSubscription{}, fmt.Errorf("data-sid attribute missing or empty")
	}

	downloadByte, err := parsePositiveInt64(attrs, "data-downloadbyte")
	if err != nil {
		return ParsedSubscription{}, fmt.Errorf("data-downloadbyte: %w", err)
	}

	uploadByte, err := parsePositiveInt64(attrs, "data-uploadbyte")
	if err != nil {
		return ParsedSubscription{}, fmt.Errorf("data-uploadbyte: %w", err)
	}

	totalByte, err := parsePositiveInt64(attrs, "data-totalbyte")
	if err != nil {
		return ParsedSubscription{}, fmt.Errorf("data-totalbyte: %w", err)
	}

	expire, err := parsePositiveInt64(attrs, "data-expire")
	if err != nil {
		return ParsedSubscription{}, fmt.Errorf("data-expire: %w", err)
	}

	// Validate field values according to requirements
	if totalByte <= 0 {
		return ParsedSubscription{}, fmt.Errorf("data-totalbyte must be positive (got %d)", totalByte)
	}

	if expire <= 0 {
		return ParsedSubscription{}, fmt.Errorf("data-expire must be positive (got %d)", expire)
	}

	if downloadByte < 0 {
		return ParsedSubscription{}, fmt.Errorf("data-downloadbyte must be non-negative (got %d)", downloadByte)
	}

	if uploadByte < 0 {
		return ParsedSubscription{}, fmt.Errorf("data-uploadbyte must be non-negative (got %d)", uploadByte)
	}

	return ParsedSubscription{
		SID:          sid,
		DownloadByte: downloadByte,
		UploadByte:   uploadByte,
		TotalByte:    totalByte,
		Expire:       expire,
	}, nil
}

// findTemplateNode recursively searches for template#subscription-data element
func findTemplateNode(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "template" {
		// Check if it has id="subscription-data"
		for _, attr := range n.Attr {
			if attr.Key == "id" && attr.Val == "subscription-data" {
				return n
			}
		}
	}

	// Recursively search children
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findTemplateNode(c); result != nil {
			return result
		}
	}

	return nil
}

// extractAttributes extracts all attributes from a node into a map
func extractAttributes(n *html.Node) map[string]string {
	attrs := make(map[string]string)
	for _, attr := range n.Attr {
		attrs[attr.Key] = attr.Val
	}
	return attrs
}

// parsePositiveInt64 parses an int64 from the attributes map
func parsePositiveInt64(attrs map[string]string, key string) (int64, error) {
	val, ok := attrs[key]
	if !ok {
		return 0, fmt.Errorf("attribute %s not found", key)
	}

	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer value: %w", err)
	}

	return parsed, nil
}
