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
package main

import (
	"log"
	"os"

	"github.com/timburks/gott/commander"
	"github.com/timburks/gott/editor"
	"github.com/timburks/gott/screen"
	gott "github.com/timburks/gott/types"
)

func main() {
	// Open a log file.
	f, err := os.OpenFile(os.Getenv("HOME")+"/.gottlog", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		log.Output(1, err.Error())
		return
	}
	log.SetOutput(f)
	defer f.Close()

	// Create a screen to manage display.
	s := screen.NewScreen()
	defer s.Close()

	// The editor manages all text manipulation.
	e := editor.NewEditor()

	// The commander converts user inputs into commands for the editor.
	c := commander.NewCommander(e)

	// If a file was specified on the command line, read it.
	if len(os.Args) > 1 {
		filename := os.Args[1]
		err = e.ReadFile(filename)
		if err != nil {
			log.Output(1, err.Error())
		}
	}

	// Run the main event loop.
	for c.GetMode() != gott.ModeQuit {
		s.Render(e, c)
		event := s.GetNextEvent()
		err = c.ProcessEvent(event)
		if err != nil {
			log.Output(1, err.Error())
		}
	}
}
