package output

import (
	"os"

	"github.com/olekukonko/tablewriter"
)

// Table wraps tablewriter for consistent table formatting across commands.
type Table struct {
	tw *tablewriter.Table
}

// NewTable creates a new table with standard Abstrax formatting.
func NewTable(headers []string) *Table {
	tw := tablewriter.NewWriter(os.Stdout)
	tw.SetHeader(headers)
	tw.SetBorder(false)
	tw.SetColumnSeparator("  ")
	tw.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAlignment(tablewriter.ALIGN_LEFT)
	tw.SetHeaderLine(false)
	tw.SetAutoFormatHeaders(false)
	tw.SetTablePadding("  ")
	tw.SetNoWhiteSpace(true)
	return &Table{tw: tw}
}

// Append adds a row.
func (t *Table) Append(row []string) {
	t.tw.Append(row)
}

// Render writes the table to stdout.
func (t *Table) Render() {
	t.tw.Render()
}
