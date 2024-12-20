package pdf2html

import (
	"encoding/xml"
	"fmt"
	"sort"
)

type PdfXmlData struct {
	XMLName  xml.Name `xml:"pdf2xml"`
	Producer *string  `xml:"producer,attr,omitempty"`
	Version  *string  `xml:"version,attr,omitempty"`

	Pages    []PdfXmlPage    `xml:"page,omitempty"`
	Outlines []PdfXmlOutline `xml:"outline,omitempty"`
}

type PdfXmlOutline struct {
	XMLName xml.Name `xml:"outline"`

	Items    []PdfXmlOutlineItem `xml:"item,omitempty"`
	Outlines []PdfXmlOutline     `xml:"outline,omitempty"`
}

type PdfXmlOutlineItem struct {
	Page    *int    `xml:"page,attr,omitempty"`
	Content *string `xml:",chardata"`
}

type PdfXmlPage struct {
	XMLName xml.Name `xml:"page"`

	PageNumber *int    `xml:"number,attr,omitempty"`
	Position   *string `xml:"position,attr,omitempty"`
	Top        *int    `xml:"top,attr,omitempty"`
	Left       *int    `xml:"left,attr,omitempty"`
	Width      *int    `xml:"width,attr,omitempty"`
	Height     *int    `xml:"height,attr,omitempty"`

	FontSpecs []PdfXmlFontSpec `xml:"fontspec,omitempty"`
	Texts     []PdfXmlText     `xml:"text,omitempty"`
}

type PdfXmlFontSpec struct {
	XMLName xml.Name `xml:"fontspec"`

	ID     *int    `xml:"id,attr"`
	Size   *int    `xml:"size,attr"`
	Family *string `xml:"family,attr"`
	Color  *string `xml:"color,attr"`
}

type PdfXmlText struct {
	XMLName xml.Name `xml:"text"`

	Top    *int `xml:"top,attr"`
	Left   *int `xml:"left,attr"`
	Width  *int `xml:"width,attr"`
	Height *int `xml:"height,attr"`

	Text     *string `xml:",chardata"`
	BoldText *string `xml:"b"`
}

type PdfXmlTableOption struct {
	From, To int // In what area should the table be located

	Columns               int // How many columns should the table have
	GetColumnFunc         func(text PdfXmlText) (int, error)
	AllowedHeightVariance int // Define what variance is allowed to be in the same line

	FilterFunc *func(entry PdfXmlTableEntry) bool // function to filter entries, if set and return true, entry will be added to the result
}

type PdfXmlTableEntry struct {
	MinLeft, MaxLeft int // Entry's minimum and maximum position left
	MinTop, MaxTop   int // Entry's minimum and maximum position top

	top int // Internal fields for validating the same line functionality

	Content []*PdfXmlTableEntryContent // the content in the text
}

type PdfXmlTableEntryContent struct {
	Text     *string // Normal text of the entry
	BoldText *string // surrounded with <b> tags text
}

type GetColumnCalculationInRangesOption struct {
	From, To int // lower and upper limit where the column should start
}

const maxInt int = int(^uint(0) >> 1)

// Extracts the table content upon a give configuration for the table
func (p PdfXmlPage) ExtractTableContent(option PdfXmlTableOption) []*PdfXmlTableEntry {
	texts := p.getSortedTexts(option.From, option.To)

	table := []*PdfXmlTableEntry{}
	for _, text := range texts {
		var entry *PdfXmlTableEntry
		if len(table) == 0 || !table[len(table)-1].isSameLine(text, option.AllowedHeightVariance) {
			entry = &PdfXmlTableEntry{
				top: *text.Top,

				MinLeft: maxInt,
				MaxLeft: 0,
				MinTop:  maxInt,
				MaxTop:  0,

				Content: make([]*PdfXmlTableEntryContent, option.Columns),
			}

			// Reset internal variables
			if len(table) != 0 {
				table[len(table)-1].top = 0
			}

			// check if old entry should stay or be removed through the filter func
			if option.FilterFunc != nil && len(table) != 0 {
				var fn func(entry PdfXmlTableEntry) bool = *option.FilterFunc

				// If last entry is NIL or it should be filtered (FilterFunc returns false) override the last entry, if not add the new
				if table[len(table)-1] == nil || !fn(*table[len(table)-1]) {
					table[len(table)-1] = entry
				} else {
					table = append(table, entry)
				}
			} else {
				table = append(table, entry)
			}
		} else {
			entry = table[len(table)-1]
		}

		column, err := option.GetColumnFunc(text)
		if err != nil {
			continue
		}

		entry.Content[column] = &PdfXmlTableEntryContent{
			Text:     text.Text,
			BoldText: text.BoldText,
		}

		// Check for min/max
		if entry.MinLeft > *text.Left {
			entry.MinLeft = *text.Left
		}
		if entry.MaxLeft < *text.Left {
			entry.MaxLeft = *text.Left
		}
		if entry.MinTop > *text.Top {
			entry.MinTop = *text.Top
		}
		if entry.MaxTop < *text.Top {
			entry.MaxTop = *text.Top
		}
	}

	if len(table) != 0 {
		// Reset internal variables
		table[len(table)-1].top = 0

		// final check after last run through
		if option.FilterFunc != nil {
			var fn func(entry PdfXmlTableEntry) bool = *option.FilterFunc

			// If last entry is NIL or it should be filtered (FilterFunc returns false) override the last entry, if not add the new
			if table[len(table)-1] == nil || !fn(*table[len(table)-1]) {
				// remove the last element
				table = table[:len(table)-1]
			}
		}
	}

	return table
}

// Provides a function upon variances around starting points the column matching
func GetColumnCalculationWithVariance(columnAveragePosition []int, allowedVariance int) func(text PdfXmlText) (int, error) {
	rangeOptions := []GetColumnCalculationInRangesOption{}
	for _, position := range columnAveragePosition {
		rangeOptions = append(rangeOptions, GetColumnCalculationInRangesOption{
			From: position - (allowedVariance / 2),
			To:   position + (allowedVariance / 2),
		})
	}

	return GetColumnCalculationInRanges(rangeOptions)
}

// Provides a function upon ranges around starting points the column matching
func GetColumnCalculationInRanges(columnRanges []GetColumnCalculationInRangesOption) func(text PdfXmlText) (int, error) {
	return func(text PdfXmlText) (int, error) {
		for i, r := range columnRanges {
			if *text.Left >= r.From && *text.Left <= r.To {
				return i, nil
			}
		}

		return -1, fmt.Errorf("cannot find correct column")
	}
}

func (e PdfXmlTableEntry) isSameLine(text PdfXmlText, variance int) bool {
	return (*text.Top - e.top) <= variance
}

func (p PdfXmlPage) getSortedTexts(from, to int) []PdfXmlText {
	texts := []PdfXmlText{}

	// Filter texts outside of the needed area
	for _, text := range p.Texts {
		if *text.Top < from || *text.Top > to {
			continue
		}

		texts = append(texts, text)
	}

	// Sort texts from top to bottom and left to right
	sort.Slice(texts, func(i, j int) bool {
		if *texts[i].Top == *texts[j].Top {
			return *texts[i].Left < *texts[j].Left
		}

		return *texts[i].Top < *texts[j].Top
	})

	return texts
}
