// Implements functions required commands on a sheet.
package sheet

import (
	"fmt"
	"strings"

	"github.com/pedroalbanese/sc/internal/evaler"
)

const (
	initial_COLUMN_WIDTH     = 10
	initial_COLUMN_PRECISION = 2
	initial_COLUMN_TYPE      = 0
)

// currentSheet is set by getDisplayValue so RangeExpander can access
// the sheet currently being evaluated.
var currentSheet *Sheet

func init() {
	evaler.RangeExpander = func(rng string) []string {
		if currentSheet == nil {
			return []string{"0"}
		}
		return currentSheet.expandRangeList(rng)
	}
}

type ColumnFormat struct {
	width     int
	precision int
	ctype     int
}

type lastEdit struct {
	address Address
	cell    *Cell
}

type Sheet struct {
	Filename string

	SelectedCell  Address
	columnFormats map[string]ColumnFormat
	data          map[Address]*Cell

	maxRowForColumn map[int]int
	maxColumnForRow map[int]int

	clipboardRangeStart, clipboardRangeEnd Address

	startRow, startCol       int
	displayRows, displayCols int

	loading bool

	lastUndo *lastEdit
}

func NewSheet(filename string) Sheet {
	s := Sheet{Filename: filename, SelectedCell: "A0",
		columnFormats: make(map[string]ColumnFormat), data: make(map[Address]*Cell),
		maxRowForColumn: make(map[int]int), maxColumnForRow: make(map[int]int)}

	s.display()
	s.Load()
	s.display()

	return s
}

func (s *Sheet) getPrecision(address Address) int {
	if cFormat, found := s.columnFormats[address.ColumnHeader()]; found {
		return cFormat.precision
	}
	return initial_COLUMN_PRECISION
}

func (s *Sheet) DecreaseColumnPrecision(column string) {
	if cFormat, found := s.columnFormats[column]; found {
		cFormat.precision--
		if cFormat.precision < 0 {
			cFormat.precision = 0
		}
		s.columnFormats[column] = cFormat
	} else {
		s.columnFormats[column] = ColumnFormat{width: initial_COLUMN_WIDTH, precision: initial_COLUMN_PRECISION - 1, ctype: initial_COLUMN_TYPE}
	}
	s.display()
}

func (s *Sheet) IncreaseColumnPrecision(column string) {
	if cFormat, found := s.columnFormats[column]; found {
		cFormat.precision++
		s.columnFormats[column] = cFormat
	} else {
		s.columnFormats[column] = ColumnFormat{width: initial_COLUMN_WIDTH, precision: initial_COLUMN_PRECISION + 1, ctype: initial_COLUMN_TYPE}
	}
	s.display()
}

func (s *Sheet) DisplayFormat(address Address) string {
	if cFormat, found := s.columnFormats[address.ColumnHeader()]; found {
		return fmt.Sprintf("%d %d %d", cFormat.width, cFormat.precision, cFormat.ctype)
	}
	return fmt.Sprintf("%d %d %d", initial_COLUMN_WIDTH, initial_COLUMN_PRECISION, initial_COLUMN_TYPE)
}

func (s *Sheet) getColumnWidth(column string) int {
	if cFormat, found := s.columnFormats[column]; found {
		return cFormat.width
	}
	s.columnFormats[column] = ColumnFormat{width: initial_COLUMN_WIDTH, precision: initial_COLUMN_PRECISION, ctype: initial_COLUMN_TYPE}
	return initial_COLUMN_WIDTH
}

func (s *Sheet) DecreaseColumnWidth(column string) {
	if cFormat, found := s.columnFormats[column]; found {
		cFormat.width--
		if cFormat.width < 1 {
			cFormat.width = 1
		}
		s.columnFormats[column] = cFormat
	} else {
		s.columnFormats[column] = ColumnFormat{width: initial_COLUMN_WIDTH - 1, precision: initial_COLUMN_PRECISION, ctype: initial_COLUMN_TYPE}
	}
	s.display()
}

func (s *Sheet) IncreaseColumnWidth(column string) {
	if cFormat, found := s.columnFormats[column]; found {
		cFormat.width++
		s.columnFormats[column] = cFormat
	} else {
		s.columnFormats[column] = ColumnFormat{width: initial_COLUMN_WIDTH + 1, precision: initial_COLUMN_PRECISION, ctype: initial_COLUMN_TYPE}
	}
	s.display()
}

func (s *Sheet) PushUndo(address Address) {
	old, err := s.GetCell(address)
	if err != nil {
		old = nil
	} else if old != nil {
		old = old.Copy(address, address)
	}
	s.lastUndo = &lastEdit{address: address, cell: old}
}

func (s *Sheet) Undo() bool {
	if s.lastUndo == nil {
		return false
	}
	entry := s.lastUndo
	s.lastUndo = nil
	if entry.cell == nil {
		delete(s.data, entry.address)
	} else {
		entry.cell.cacheValid = false
		s.data[entry.address] = entry.cell
	}
	s.invalidateCache(entry.address)
	s.display()
	return true
}

func (s *Sheet) invalidateCache(address Address) {
	if cell, err := s.GetCell(address); err == nil && cell != nil {
		cell.cacheValid = false
	}
	for addr, cell := range s.data {
		if _, depends := cell.forwardRefs[address]; depends {
			if cell.cacheValid {
				cell.cacheValid = false
				s.invalidateCache(addr)
			}
		}
	}
}

func (s *Sheet) ClearCell(address Address) {
	if !s.loading {
		s.PushUndo(address)
	}
	delete(s.data, address)
	s.findMaximums(address)
	s.invalidateCache(address)
	s.display()
}

func (s *Sheet) GetCell(address Address) (*Cell, error) {
	if cell, found := s.data[address]; found {
		return cell, nil
	} else if address == s.SelectedCell {
		return &Cell{}, nil
	}
	return nil, &ErrCellNotFound{address}
}

func (s *Sheet) SetCell(address Address, cell *Cell) {
	if !s.loading {
		s.PushUndo(address)
	}
	if !cell.stringType {
		postfix := evaler.GetPostfix(cell.value)
		for _, token := range postfix {
			if evaler.IsCellRange(token) {
				parts := strings.SplitN(token, ":", 2)
				start := Address(parts[0])
				end := Address(parts[1])
				r1, c1 := start.RowCol()
				r2, c2 := end.RowCol()
				for r := r1; r <= r2; r++ {
					for c := c1; c <= c2; c++ {
						cell.forwardRefs[NewAddress(r, c)] = struct{}{}
					}
				}
			} else if evaler.IsCellAddr(token) {
				cell.forwardRefs[Address(token)] = struct{}{}
			}
		}
	}
	s.data[address] = cell
	s.setMaximums(address)
	s.invalidateCache(address)
	if !s.loading {
		s.display()
	}
}

func (s *Sheet) setMaximums(address Address) {
	row, col := address.RowCol()
	if row > s.maxRowForColumn[col] {
		s.maxRowForColumn[col] = row
	}
	if col > s.maxColumnForRow[row] {
		s.maxColumnForRow[row] = col
	}
}

func (s *Sheet) findMaximums(address Address) {
	row, column := address.RowCol()
	if column == s.maxColumnForRow[row] {
		delete(s.maxColumnForRow, row)
		for colIdx := column; colIdx >= 0; colIdx-- {
			if _, found := s.data[NewAddress(row, colIdx)]; found {
				s.maxColumnForRow[row] = colIdx
			}
		}
	}
	if row == s.maxRowForColumn[column] {
		delete(s.maxRowForColumn, column)
		for rowIdx := row; rowIdx >= 0; rowIdx-- {
			if _, found := s.data[NewAddress(rowIdx, column)]; found {
				s.maxRowForColumn[column] = rowIdx
			}
		}
	}
}
