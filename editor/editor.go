//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
package editor

import (
	"io/ioutil"
	"os"
	"strings"
	"unicode"

	gott "github.com/timburks/gott/types"
)

// The Editor manages the editing of text in a Buffer.
type Editor struct {
	cursor    gott.Point           // cursor position
	offset    gott.Size            // display offset
	buffer    *Buffer              // active buffer being edited
	size      gott.Size            // size of editing area
	pasteText string               // used to cut/copy and paste
	pasteMode int                  // how to paste the string on the pasteboard
	previous  gott.Operation       // last operation performed, available to repeat
	undo      []gott.Operation     // stack of operations to undo
	insert    gott.InsertOperation // when in insert mode, the current insert operation
}

func NewEditor() *Editor {
	e := &Editor{}
	e.buffer = NewBuffer()
	return e
}

func (e *Editor) ReadFile(path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	e.buffer.ReadBytes(b)
	e.buffer.FileName = path
	return nil
}

func (e *Editor) Bytes() []byte {
	return e.buffer.Bytes()
}

func (e *Editor) WriteFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b := e.Bytes()
	if strings.HasSuffix(path, ".go") {
		out, err := e.Gofmt(e.buffer.FileName, b)
		if err == nil {
			f.Write(out)
		} else {
			f.Write(b)
		}
	} else {
		f.Write(b)
	}
	return nil
}

func (e *Editor) Perform(op gott.Operation, multiplier int) {
	// perform the operation
	inverse := op.Perform(e, multiplier)
	// save the operation for repeats
	e.previous = op
	// save the inverse of the operation for undo
	if inverse != nil {
		e.undo = append(e.undo, inverse)
	}
}

func (e *Editor) Repeat() {
	if e.previous != nil {
		inverse := e.previous.Perform(e, 0)
		if inverse != nil {
			e.undo = append(e.undo, inverse)
		}
	}
}

func (e *Editor) PerformUndo() {
	if len(e.undo) > 0 {
		last := len(e.undo) - 1
		undo := e.undo[last]
		e.undo = e.undo[0:last]
		undo.Perform(e, 0)
	}
}

func (e *Editor) PerformSearch(text string) {
	if e.buffer.GetRowCount() == 0 {
		return
	}
	row := e.cursor.Row
	col := e.cursor.Col + 1

	for {
		var s string
		if col < e.buffer.GetRowLength(row) {
			s = e.buffer.TextAfter(row, col)
		} else {
			s = ""
		}
		i := strings.Index(s, text)
		if i != -1 {
			// found it
			e.cursor.Row = row
			e.cursor.Col = col + i
			return
		} else {
			col = 0
			row = row + 1
			if row == e.buffer.GetRowCount() {
				row = 0
			}
		}
		if row == e.cursor.Row {
			break
		}
	}
}

func (e *Editor) Scroll() {
	if e.cursor.Row < e.offset.Rows {
		e.offset.Rows = e.cursor.Row
	}
	if e.cursor.Row-e.offset.Rows >= e.size.Rows {
		e.offset.Rows = e.cursor.Row - e.size.Rows + 1
	}
	if e.cursor.Col < e.offset.Cols {
		e.offset.Cols = e.cursor.Col
	}
	if e.cursor.Col-e.offset.Cols >= e.size.Cols {
		e.offset.Cols = e.cursor.Col - e.size.Cols + 1
	}
}

func (e *Editor) MoveCursor(direction int) {
	switch direction {
	case gott.MoveLeft:
		if e.cursor.Col > 0 {
			e.cursor.Col--
		}
	case gott.MoveRight:
		if e.cursor.Row < e.buffer.GetRowCount() {
			rowLength := e.buffer.GetRowLength(e.cursor.Row)
			if e.cursor.Col < rowLength-1 {
				e.cursor.Col++
			}
		}
	case gott.MoveUp:
		if e.cursor.Row > 0 {
			e.cursor.Row--
		}
	case gott.MoveDown:
		if e.cursor.Row < e.buffer.GetRowCount()-1 {
			e.cursor.Row++
		}
	}
	// don't go past the end of the current line
	if e.cursor.Row < e.buffer.GetRowCount() {
		rowLength := e.buffer.GetRowLength(e.cursor.Row)
		if e.cursor.Col > rowLength-1 {
			e.cursor.Col = rowLength - 1
			if e.cursor.Col < 0 {
				e.cursor.Col = 0
			}
		}
	}
}

// These editor primitives will make changes in insert mode and associate them with to the current operation.

func (e *Editor) InsertChar(c rune) {
	if e.insert != nil {
		e.insert.AddCharacter(c)
	}
	if c == '\n' {
		e.InsertRow()
		e.cursor.Row++
		e.cursor.Col = 0
		return
	}
	// if the cursor is past the nmber of rows, add a row
	for e.cursor.Row >= e.buffer.GetRowCount() {
		e.AppendBlankRow()
	}
	e.buffer.InsertCharacter(e.cursor.Row, e.cursor.Col, c)
	e.cursor.Col += 1
}

func (e *Editor) InsertRow() {
	if e.cursor.Row >= e.buffer.GetRowCount() {
		// we should never get here
		e.AppendBlankRow()
	} else {
		newRow := e.buffer.rows[e.cursor.Row].Split(e.cursor.Col)
		i := e.cursor.Row + 1
		// add a dummy row at the end of the Rows slice
		e.AppendBlankRow()
		// move rows to make room for the one we are adding
		copy(e.buffer.rows[i+1:], e.buffer.rows[i:])
		// add the new row
		e.buffer.rows[i] = newRow
	}
}

func (e *Editor) BackspaceChar() rune {
	if e.buffer.GetRowCount() == 0 {
		return rune(0)
	}
	if e.insert.Length() == 0 {
		return rune(0)
	}
	e.insert.DeleteCharacter()
	if e.cursor.Col > 0 {
		c := e.buffer.rows[e.cursor.Row].DeleteChar(e.cursor.Col - 1)
		e.cursor.Col--
		return c
	} else if e.cursor.Row > 0 {
		// remove the current row and join it with the previous one
		oldRowText := e.buffer.rows[e.cursor.Row].Text
		var newCursor gott.Point
		newCursor.Col = len(e.buffer.rows[e.cursor.Row-1].Text)
		e.buffer.rows[e.cursor.Row-1].Text = append(e.buffer.rows[e.cursor.Row-1].Text, oldRowText...)
		e.buffer.rows = append(e.buffer.rows[0:e.cursor.Row], e.buffer.rows[e.cursor.Row+1:]...)
		e.cursor.Row--
		e.cursor.Col = newCursor.Col
		return rune('\n')
	} else {
		return rune(0)
	}
}

func (e *Editor) JoinRow(multiplier int) []gott.Point {
	if e.buffer.GetRowCount() == 0 {
		return nil
	}
	// remove the next row and join it with this one
	insertions := make([]gott.Point, 0)
	for i := 0; i < multiplier; i++ {
		oldRowText := e.buffer.rows[e.cursor.Row+1].Text
		var newCursor gott.Point
		newCursor.Col = len(e.buffer.rows[e.cursor.Row].Text)
		e.buffer.rows[e.cursor.Row].Text = append(e.buffer.rows[e.cursor.Row].Text, oldRowText...)
		e.buffer.rows = append(e.buffer.rows[0:e.cursor.Row+1], e.buffer.rows[e.cursor.Row+2:]...)
		e.cursor.Col = newCursor.Col
		insertions = append(insertions, e.cursor)
	}
	return insertions
}

func (e *Editor) YankRow(multiplier int) {
	if e.buffer.GetRowCount() == 0 {
		return
	}
	pasteText := ""
	for i := 0; i < multiplier; i++ {
		position := e.cursor.Row + i
		if position < e.buffer.GetRowCount() {
			pasteText += string(e.buffer.rows[position].Text) + "\n"
		}
	}

	e.SetPasteBoard(pasteText, gott.PasteNewLine)
}

func (e *Editor) KeepCursorInRow() {
	if e.buffer.GetRowCount() == 0 {
		e.cursor.Col = 0
	} else {
		if e.cursor.Row >= e.buffer.GetRowCount() {
			e.cursor.Row = e.buffer.GetRowCount() - 1
		}
		if e.cursor.Row < 0 {
			e.cursor.Row = 0
		}
		lastIndexInRow := e.buffer.rows[e.cursor.Row].Length() - 1
		if e.cursor.Col > lastIndexInRow {
			e.cursor.Col = lastIndexInRow
		}
		if e.cursor.Col < 0 {
			e.cursor.Col = 0
		}
	}
}

func (e *Editor) AppendBlankRow() {
	e.buffer.rows = append(e.buffer.rows, NewRow(""))
}

func (e *Editor) InsertLineAboveCursor() {
	e.AppendBlankRow()
	copy(e.buffer.rows[e.cursor.Row+1:], e.buffer.rows[e.cursor.Row:])
	e.buffer.rows[e.cursor.Row] = NewRow("")
	e.cursor.Col = 0
}

func (e *Editor) InsertLineBelowCursor() {
	e.AppendBlankRow()
	copy(e.buffer.rows[e.cursor.Row+2:], e.buffer.rows[e.cursor.Row+1:])
	e.buffer.rows[e.cursor.Row+1] = NewRow("")
	e.cursor.Row += 1
	e.cursor.Col = 0
}

func (e *Editor) MoveCursorToStartOfLine() {
	e.cursor.Col = 0
}

func (e *Editor) MoveCursorToStartOfLineBelowCursor() {
	e.cursor.Col = 0
	e.cursor.Row += 1
}

// editable

func (e *Editor) GetCursor() gott.Point {
	return e.cursor
}

func (e *Editor) SetCursor(cursor gott.Point) {
	e.cursor = cursor
}

func (e *Editor) ReplaceCharacterAtCursor(cursor gott.Point, c rune) rune {
	return e.buffer.rows[cursor.Row].ReplaceChar(cursor.Col, c)
}

func (e *Editor) DeleteRowsAtCursor(multiplier int) string {
	deletedText := ""
	for i := 0; i < multiplier; i++ {
		row := e.cursor.Row
		if row < e.buffer.GetRowCount() {
			if i > 0 {
				deletedText += "\n"
			}
			deletedText += string(e.buffer.rows[row].Text)
			e.buffer.rows = append(e.buffer.rows[0:row], e.buffer.rows[row+1:]...)
		} else {
			break
		}
	}
	e.cursor.Row = clipToRange(e.cursor.Row, 0, e.buffer.GetRowCount()-1)
	return deletedText
}

func (e *Editor) SetPasteBoard(text string, mode int) {
	e.pasteText = text
	e.pasteMode = mode
}

func (e *Editor) DeleteWordsAtCursor(multiplier int) string {
	deletedText := ""
	for i := 0; i < multiplier; i++ {
		if e.buffer.GetRowCount() == 0 {
			break
		}
		// if the row is empty, delete the row...
		row := e.cursor.Row
		col := e.cursor.Col
		b := e.buffer
		if col >= b.rows[row].Length() {
			position := e.cursor.Row
			e.buffer.rows = append(e.buffer.rows[0:position], e.buffer.rows[position+1:]...)
			deletedText += "\n"
			e.KeepCursorInRow()
		} else {
			// else do this...
			c := e.buffer.rows[e.cursor.Row].DeleteChar(e.cursor.Col)
			deletedText += string(c)
			for {
				if e.cursor.Col > e.buffer.rows[e.cursor.Row].Length()-1 {
					break
				}
				if c == ' ' {
					break
				}
				c = e.buffer.rows[e.cursor.Row].DeleteChar(e.cursor.Col)
				deletedText += string(c)
			}
			if e.cursor.Col > e.buffer.rows[e.cursor.Row].Length()-1 {
				e.cursor.Col--
			}
			if e.cursor.Col < 0 {
				e.cursor.Col = 0
			}
		}
	}
	return deletedText
}

func (e *Editor) DeleteCharactersAtCursor(multiplier int, undo bool, finallyDeleteRow bool) string {
	deletedText := e.buffer.DeleteCharacters(e.cursor.Row, e.cursor.Col, multiplier, undo)
	if e.cursor.Col > e.buffer.rows[e.cursor.Row].Length()-1 {
		e.cursor.Col--
	}
	if e.cursor.Col < 0 {
		e.cursor.Col = 0
	}
	if finallyDeleteRow && e.buffer.GetRowCount() > 0 {
		e.buffer.DeleteRow(e.cursor.Row)
	}
	return deletedText
}

func (e *Editor) ChangeWordAtCursor(multiplier int, text string) (string, int) {
	// delete the next N words and enter insert mode.
	deletedText := e.DeleteWordsAtCursor(multiplier)

	var mode int
	if text != "" { // repeat
		r := e.cursor.Row
		c := e.cursor.Col
		for _, c := range text {
			e.InsertChar(c)
		}
		e.cursor.Row = r
		e.cursor.Col = c
		mode = gott.ModeEdit
	} else {
		mode = gott.ModeInsert
	}

	return deletedText, mode
}

func (e *Editor) InsertText(text string, position int) (gott.Point, int) {
	if e.buffer.GetRowCount() == 0 {
		e.AppendBlankRow()
	}
	switch position {
	case gott.InsertAtCursor:
		break
	case gott.InsertAfterCursor:
		e.cursor.Col++
		e.cursor.Col = clipToRange(e.cursor.Col, 0, e.buffer.rows[e.cursor.Row].Length())
	case gott.InsertAtStartOfLine:
		e.cursor.Col = 0
	case gott.InsertAfterEndOfLine:
		e.cursor.Col = e.buffer.rows[e.cursor.Row].Length()
	case gott.InsertAtNewLineBelowCursor:
		e.InsertLineBelowCursor()
	case gott.InsertAtNewLineAboveCursor:
		e.InsertLineAboveCursor()
	}
	var mode int
	if text != "" {
		r := e.cursor.Row
		c := e.cursor.Col
		for _, c := range text {
			e.InsertChar(c)
		}
		e.cursor.Row = r
		e.cursor.Col = c
		mode = gott.ModeEdit
	} else {
		mode = gott.ModeInsert
	}
	return e.cursor, mode
}

func (e *Editor) SetInsertOperation(insert gott.InsertOperation) {
	e.insert = insert
}

func (e *Editor) GetPasteMode() int {
	return e.pasteMode
}

func (e *Editor) GetPasteText() string {
	return e.pasteText
}

func (e *Editor) ReverseCaseCharactersAtCursor(multiplier int) {
	if e.buffer.GetRowCount() == 0 {
		return
	}
	row := &e.buffer.rows[e.cursor.Row]
	for i := 0; i < multiplier; i++ {
		c := row.Text[e.cursor.Col]
		if unicode.IsUpper(c) {
			row.ReplaceChar(e.cursor.Col, unicode.ToLower(c))
		}
		if unicode.IsLower(c) {
			row.ReplaceChar(e.cursor.Col, unicode.ToUpper(c))
		}
		if e.cursor.Col < row.Length()-1 {
			e.cursor.Col++
		}
	}
}

func (e *Editor) PageUp() {
	// move to the top of the screen
	e.cursor.Row = e.offset.Rows
	// move up by a page
	for i := 0; i < e.size.Rows; i++ {
		e.MoveCursor(gott.MoveUp)
	}
}

func (e *Editor) PageDown() {
	// move to the bottom of the screen
	e.cursor.Row = e.offset.Rows + e.size.Rows - 1
	// move down by a page
	for i := 0; i < e.size.Rows; i++ {
		e.MoveCursor(gott.MoveDown)
	}
}

func (e *Editor) SetSize(s gott.Size) {
	e.size = s
}

func (e *Editor) CloseInsert() {
	e.insert.Close()
	e.insert = nil
}

func (e *Editor) MoveToBeginningOfLine() {
	e.cursor.Col = 0
}

func (e *Editor) MoveToEndOfLine() {
	e.cursor.Col = 0
	if e.cursor.Row < e.buffer.GetRowCount() {
		e.cursor.Col = e.buffer.GetRowLength(e.cursor.Row) - 1
		if e.cursor.Col < 0 {
			e.cursor.Col = 0
		}
	}
}

func (e *Editor) GetBuffer() gott.Buffer {
	return e.buffer
}

func (e *Editor) GetOffset() gott.Size {
	return e.offset
}
