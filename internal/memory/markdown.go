package memory

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func formatMarkdown(category Category, entries []Entry) string {
	var builder strings.Builder
	builder.WriteString("# GoClaw ")
	builder.WriteString(string(category))
	builder.WriteString(" memory\n\n")
	builder.WriteString("<!-- goclaw-memory:v1 -->\n\n")
	for _, entry := range entries {
		builder.WriteString("## ")
		builder.WriteString(entry.ID)
		builder.WriteByte('\n')
		builder.WriteString("created_at: ")
		builder.WriteString(entry.CreatedAt.UTC().Format(time.RFC3339))
		builder.WriteByte('\n')
		builder.WriteString("updated_at: ")
		builder.WriteString(entry.UpdatedAt.UTC().Format(time.RFC3339))
		builder.WriteByte('\n')
		builder.WriteString("content: ")
		builder.WriteString(strconv.Quote(entry.Content))
		builder.WriteString("\n\n")
	}
	return builder.String()
}

func parseMarkdown(category Category, data string) ([]Entry, error) {
	var entries []Entry
	sections := strings.Split(data, "\n## ")
	for index, section := range sections {
		if index == 0 {
			continue
		}
		entry, err := parseSection(category, strings.TrimSpace(section))
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseSection(category Category, section string) (Entry, error) {
	lines := strings.Split(section, "\n")
	if len(lines) < 4 {
		return Entry{}, fmt.Errorf("decode memory markdown: malformed entry")
	}
	entry := Entry{
		ID:       strings.TrimSpace(lines[0]),
		Category: category,
	}
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "created_at":
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return Entry{}, fmt.Errorf("decode memory created_at: %w", err)
			}
			entry.CreatedAt = parsed
		case "updated_at":
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return Entry{}, fmt.Errorf("decode memory updated_at: %w", err)
			}
			entry.UpdatedAt = parsed
		case "content":
			content, err := strconv.Unquote(value)
			if err != nil {
				return Entry{}, fmt.Errorf("decode memory content: %w", err)
			}
			entry.Content = content
		}
	}
	if err := entry.Validate(); err != nil {
		return Entry{}, fmt.Errorf("validate memory entry: %w", err)
	}
	return entry, nil
}
