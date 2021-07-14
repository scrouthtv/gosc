package sheet

import "fmt"

type ErrCellNotFound struct {
	Addr Address
}

func (e *ErrCellNotFound) Error() string {
	return fmt.Sprintf("cell at address %s does not exist in spreadsheet", e.Addr)
}
