package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func extractColumnItems(content string, columnName string) ([]string, error) {
	parser := goldmark.DefaultParser()
	reader := text.NewReader([]byte(content))
	doc := parser.Parse(reader)

	var items []string

	var foundTargetHeading bool
	var currentHeadingLevel int

	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			headingLevel := node.Level

			if foundTargetHeading && headingLevel <= currentHeadingLevel {
				return ast.WalkStop, nil
			}

			if headingLevel == 2 {
				headingText := string(node.Text([]byte(content)))
				if strings.TrimSpace(headingText) == columnName {
					foundTargetHeading = true
					currentHeadingLevel = headingLevel
				}
			}

		case *ast.ListItem:
			if foundTargetHeading {
				listItemText := string(node.Text([]byte(content)))

				if strings.Contains(listItemText, "[") && strings.Contains(listItemText, "]") {
					checkboxIndex := strings.Index(listItemText, "]")
					if checkboxIndex >= 0 && checkboxIndex+1 < len(listItemText) {
						itemText := strings.Join(strings.Fields(listItemText[checkboxIndex+1:]), " ")
						if itemText != "" {
							items = append(items, itemText)
						}
					}
				}
			}
		}

		return ast.WalkContinue, nil
	})

	if err != nil {
		return nil, err
	}

	if !foundTargetHeading {
		return nil, fmt.Errorf("column '%s' not found", columnName)
	}

	return items, nil
}

func categorizeByTags(titles []string) map[string][]string {
	categories := map[string][]string{
		"features":        {},
		"bugs":            {},
		"planning/design": {},
		"documentation":   {},
		"reviews":         {},
		"learning":        {},
		"other":           {},
	}

	for _, title := range titles {
		// Extract all hashtags from the title
		words := strings.Fields(title)
		var tags []string
		for _, word := range words {
			if strings.HasPrefix(word, "#") {
				tags = append(tags, strings.ToLower(strings.TrimPrefix(word, "#")))
			}
		}

		// If no tags found, put in other category
		if len(tags) == 0 {
			categories["other"] = append(categories["other"], title)
			continue
		}

		// Use the first matching tag to categorize
		categorized := false
		for _, tag := range tags {
			switch tag {
			case "build", "feat", "feature":
				categories["features"] = append(categories["features"], title)
				categorized = true
			case "bug":
				categories["bugs"] = append(categories["bugs"], title)
				categorized = true
			case "plan", "design":
				categories["planning/design"] = append(categories["planning/design"], title)
				categorized = true
			case "doc", "docs":
				categories["documentation"] = append(categories["documentation"], title)
				categorized = true
			case "review":
				categories["reviews"] = append(categories["reviews"], title)
				categorized = true
			case "learn":
				categories["learning"] = append(categories["learning"], title)
				categorized = true
			}
			if categorized {
				break
			}
		}

		if !categorized {
			categories["other"] = append(categories["other"], title)
		}
	}

	return categories
}

func summarizeByCategory(categories map[string][]string, apiKey string) (map[string][]string, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	client := openai.NewClient(apiKey)
	ctx := context.Background()
	result := make(map[string][]string)

	for category, titles := range categories {
		if len(titles) == 0 {
			continue
		}

		itemsList := strings.Join(titles, "\n- ")
		prompt := fmt.Sprintf(`As an expert software engineer with strong communication skills, write a concise technical summary of the following items in the '%s' category. 
Focus on technical impact, architectural decisions, and engineering outcomes. Write in a clear, professional tone suitable for team communication or management updates.
Keep it brief but informative, highlighting key technical achievements and challenges.

Items to summarize:
%s

Format your response as a brief technical summary paragraph, followed by key bullet points if needed.`, category, itemsList)

		resp, err := client.CreateChatCompletion(
			ctx,
			openai.ChatCompletionRequest{
				Model: "gpt-4o-mini",
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleUser,
						Content: prompt,
					},
				},
				MaxTokens: 500,
			},
		)

		if err != nil {
			return nil, fmt.Errorf("error calling OpenAI API for category '%s': %w", category, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no response from OpenAI API for category '%s'", category)
		}

		responseText := resp.Choices[0].Message.Content
		bullets := extractBulletPoints(responseText)

		if len(bullets) == 0 {
			log.Printf("WARNING: Empty summary received for category '%s'", category)
		}

		result[category] = bullets
	}

	return result, nil
}

func extractBulletPoints(text string) []string {
	lines := strings.Split(text, "\n")
	var bullets []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "• ") ||
			(len(line) >= 2 && line[0] >= '1' && line[0] <= '9' && line[1] == '.') {
			cleanLine := line
			if strings.HasPrefix(line, "- ") {
				cleanLine = line[2:]
			} else if strings.HasPrefix(line, "• ") {
				cleanLine = line[2:]
			} else if len(line) >= 2 && line[0] >= '1' && line[0] <= '9' && line[1] == '.' {
				parts := strings.SplitN(line, ". ", 2)
				if len(parts) > 1 {
					cleanLine = parts[1]
				}
			}

			cleanLine = strings.TrimSpace(cleanLine)
			if cleanLine != "" {
				bullets = append(bullets, cleanLine)
			}
		}
	}

	return bullets
}

func buildMarkdownSummary(summaries map[string][]string, year int, week int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Week %d %d\n\n", week, year))

	for category, bullets := range summaries {
		if len(bullets) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", strings.Title(category)))

		// First bullet is the summary paragraph
		if len(bullets) > 0 {
			sb.WriteString(bullets[0])
			sb.WriteString("\n\n")
		}

		// Remaining bullets are key points
		if len(bullets) > 1 {
			sb.WriteString("**Key Points:**\n")
			for _, bullet := range bullets[1:] {
				sb.WriteString(fmt.Sprintf("- %s\n", bullet))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func saveWorklog(outputFolder string, year int, week int, content string) error {
	err := os.MkdirAll(outputFolder, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output folder: %w", err)
	}

	filename := fmt.Sprintf("%s/worklog-week-%d-%d.md", outputFolder, week, year)

	err = os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write worklog file: %w", err)
	}

	log.Printf("INFO: Saved worklog to %s", filename)
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("WORKLOG-GEN: ")

	boardPath := flag.String("board", "", "Path to the Kanban board markdown file")
	column := flag.String("column", "", "Column to summarize")
	outputFolder := flag.String("output-folder", "", "Folder to write the summary")
	apiKey := flag.String("api-key", "", "OpenAI API key (can also be set via OPENAI_API_KEY env var)")

	flag.Parse()

	if *boardPath == "" || *column == "" || *outputFolder == "" {
		log.Println("ERROR: board, column, and output-folder flags are required")
		flag.Usage()
		os.Exit(1)
	}

	_, err := os.Stat(*boardPath)
	if os.IsNotExist(err) {
		log.Fatalf("ERROR: Board file '%s' does not exist", *boardPath)
	}

	log.Printf("INFO: Reading board file: %s", *boardPath)
	data, err := os.ReadFile(*boardPath)
	if err != nil {
		log.Fatalf("ERROR: Failed to read board file: %v", err)
	}
	boardMarkdown := string(data)

	log.Printf("INFO: Extracting items from column: %s", *column)
	items, err := extractColumnItems(boardMarkdown, *column)
	if err != nil {
		log.Fatalf("ERROR: %v", err)
	}

	if len(items) == 0 {
		log.Println("WARNING: No cards found in the specified column")
	} else {
		log.Printf("INFO: Found %d cards in column '%s'", len(items), *column)
	}

	categories := categorizeByTags(items)

	if *apiKey == "" {
		*apiKey = os.Getenv("OPENAI_API_KEY")
		if *apiKey == "" {
			log.Println("ERROR: No OpenAI API key provided. Skipping summary generation.")
			os.Exit(1)
		}
	}

	log.Println("INFO: Generating summaries using OpenAI API")
	summaries, err := summarizeByCategory(categories, *apiKey)
	if err != nil {
		log.Fatalf("ERROR: Failed to generate summaries: %v", err)
	}

	hasAnySummaries := false
	for _, bullets := range summaries {
		if len(bullets) > 0 {
			hasAnySummaries = true
			break
		}
	}

	if !hasAnySummaries {
		log.Println("WARNING: All summaries are empty")
	}

	currentYear := time.Now().Year()
	_, currentWeek := time.Now().ISOWeek()

	log.Printf("INFO: Building worklog summary for week %d, %d", currentWeek, currentYear)
	summary := buildMarkdownSummary(summaries, currentYear, currentWeek)

	err = saveWorklog(*outputFolder, currentYear, currentWeek, summary)
	if err != nil {
		log.Fatalf("ERROR: Failed to save worklog: %v", err)
	}

	totalItems := 0
	for _, items := range categories {
		totalItems += len(items)
	}

	worklogFilename := fmt.Sprintf("worklog-week-%d-%d.md", currentWeek, currentYear)
	log.Printf("SUCCESS: Summarized %d items to %s/%s", totalItems, *outputFolder, worklogFilename)
}
