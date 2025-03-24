# Obsidian Worklog Generator

This tool extracts checklist items from specified columns in Markdown-based Kanban boards and generates a summary via OpenAI's gpt-4o-mini. It's designed to work well with Obsidian Kanban boards.

## Features

- Extract all checklist items (`- [ ]`) from a specific column
- Uses goldmark for robust Markdown parsing
- Generate formatted output files

## Installation

```bash
git clone https://github.com/yourusername/obsidian-worklog-gen.git
cd obsidian-worklog-gen
go build
```

## Usage

```bash
./obsidian-worklog-gen --board=/path/to/board.md --column="Done" --output-folder=./output
```

### Command-line Arguments

- `--board`: Path to your Kanban board Markdown file
- `--column`: Name of the column to extract items from (case-sensitive, must match exactly)
- `--output-folder`: Directory where the output file should be created

## Output

The program creates a file named `{column}_items.txt` in the specified output folder, containing a numbered list of all the checklist items from the chosen column.

## Implementation Details

This tool uses the [goldmark](https://github.com/yuin/goldmark) library for proper Markdown parsing, providing robust handling of Markdown documents even with complex formatting. 