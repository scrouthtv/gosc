package sheet

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/pedroalbanese/sc/internal/display"
	"github.com/pedroalbanese/sc/internal/evaler"
	"github.com/pedroalbanese/sc/internal/sheet/align"
)

type Cell struct {
	value      string
	stringType bool
	alignment  align.Align

	forwardRefs map[Address]struct{}

	cachedValue string
	cacheValid  bool
	calculating bool
}

func NewCell(value string, alignment align.Align, stringType bool) *Cell {
	return &Cell{
		value:       value,
		alignment:   alignment,
		stringType:  stringType,
		forwardRefs: make(map[Address]struct{}),
		cacheValid:  false,
	}
}

func (c *Cell) Copy(oldAddress, newAddress Address) *Cell {
	val := c.value
	if !c.stringType {
		rowDiff := newAddress.Row() - oldAddress.Row()
		colDiff := newAddress.Column() - oldAddress.Column()
		for forRef := range c.forwardRefs {
			refRow, refCol := forRef.RowCol()
			newRef := NewAddress(refRow+rowDiff, refCol+colDiff)
			val = strings.Replace(val, string(forRef), string(newRef), -1)
		}
	}
	return NewCell(val, c.alignment, c.stringType)
}

func (c *Cell) getDisplayValue(s *Sheet, address Address) string {
	if c.cacheValid {
		return c.cachedValue
	}
	if c.calculating {
		return "circle"
	}
	c.calculating = true
	defer func() { c.calculating = false }()

	// Register the current sheet so RangeExpander can access it.
	prevSheet := currentSheet
	currentSheet = s
	defer func() { currentSheet = prevSheet }()

	postfix := evaler.GetPostfix(c.value)

	// Substitute cell addresses with their values. Ranges are left as
	// tokens; the evaler expands them via RangeExpander.
	for idx, token := range postfix {
		if evaler.IsCellAddr(token) {
			refCellVal := "0"
			if cell, err := s.GetCell(Address(token)); err == nil && cell != nil {
				v := cell.getDisplayValue(s, Address(token))
				if v == "circle" {
					c.cachedValue = "circle"
					c.cacheValid = true
					return "circle"
				}
				if v != "" {
					refCellVal = v
				}
			}
			postfix[idx] = refCellVal
		}
	}

	result := ""
	if rat, err := evaler.EvaluatePostfix(postfix); err == nil {
		result = rat.FloatString(s.getPrecision(address))
	}
	c.cachedValue = result
	c.cacheValid = true
	return result
}

// expandRangeList expands a range like A1:B3 into a list of cell values.
// This is used by the evaler's RangeExpander.
func (s *Sheet) expandRangeList(rng string) []string {
	parts := strings.SplitN(rng, ":", 2)
	if len(parts) != 2 {
		return []string{"0"}
	}
	start := Address(parts[0])
	end := Address(parts[1])
	r1, c1 := start.RowCol()
	r2, c2 := end.RowCol()

	if r1 > r2 {
		r1, r2 = r2, r1
	}
	if c1 > c2 {
		c1, c2 = c2, c1
	}

	var vals []string
	for r := r1; r <= r2; r++ {
		for c := c1; c <= c2; c++ {
			addr := NewAddress(r, c)
			cell, err := s.GetCell(addr)
			if err != nil || cell == nil || cell.stringType {
				continue
			}
			v := cell.getDisplayValue(s, addr)
			if v == "circle" {
				return []string{"0"}
			}
			if v == "" {
				v = "0"
			}
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return []string{"0"}
	}
	return vals
}

func (c *Cell) display(s *Sheet, address Address, row, colStart, colEnd int, selected bool) {
	dispVal := c.getDisplay(s, address)
	display.DisplayValue(dispVal, row, colStart, colEnd, c.alignment, selected)
}

func (c *Cell) getDisplay(s *Sheet, addr Address) string {
	if c.stringType {
		return c.value
	}
	return c.getDisplayValue(s, addr)
}

func (c *Cell) StatusBarVal() string {
	if c.stringType {
		modifier := ""
		switch c.alignment {
		case align.AlignLeft:
			modifier = "<"
		case align.AlignCenter:
			modifier = "|"
		default:
			modifier = ">"
		}
		return fmt.Sprintf("%s\"%s\"", modifier, c.value)
	}
	return c.value
}

// expandRange sums a range like A1:B3. Used only for formulas without
// functions; when a function is present, the evaler expands the range
// into individual values via RangeExpander.
func (s *Sheet) expandRange(rng string) string {
	parts := strings.SplitN(rng, ":", 2)
	if len(parts) != 2 {
		return "0"
	}
	start := Address(parts[0])
	end := Address(parts[1])
	r1, c1 := start.RowCol()
	r2, c2 := end.RowCol()

	sum := new(big.Rat)
	for r := r1; r <= r2; r++ {
		for c := c1; c <= c2; c++ {
			addr := NewAddress(r, c)
			cell, err := s.GetCell(addr)
			if err != nil || cell == nil || cell.stringType {
				continue
			}
			valStr := cell.getDisplayValue(s, addr)
			if valStr == "circle" {
				return "circle"
			}
			if valStr == "" {
				continue
			}
			v := new(big.Rat)
			if _, err := fmt.Sscan(valStr, v); err == nil {
				sum.Add(sum, v)
			}
		}
	}
	return sum.FloatString(10)
}
