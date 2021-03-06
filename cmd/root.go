/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"

	"github.com/goccy/go-graphviz"
	"github.com/pkg/errors"

	"github.com/spf13/cobra"
)

var outputFile string
var format string
var gFormat graphviz.Format
var watch bool
var palette []string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ketodot",
	Short: "Draws a relation graph based on Ory Keto relation-tuples definitions",
	Long: `Ketodot is small program that takes a file of relation tuples in Ory keto syntax,
and generates the resulting graph of relations. It can either output a graphviz
DOT syntax, or directly an image file. It can also watch a source file for
changes and adjust the output accordingly for easy development.`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	RunE: func(cmd *cobra.Command, args []string) error {
		initOutput()
		cleanup()

		if watch {
			watcher, err := fsnotify.NewWatcher()
			done := make(chan bool)
			if err != nil {
				log.Fatal(err)
			}
			defer watcher.Close()
			go watchSave(cmd, args, watcher)
			for _, v := range args {
				err := watcher.Add(v)
				if err != nil {
					log.Fatal(err)
				}
			}
			oneLoop(cmd, args)
			cleanup()
			<-done
		} else {
			oneLoop(cmd, args)
		}

		return nil
	},
}

func oneLoop(cmd *cobra.Command, args []string) {
	rts := parse(cmd, args)
	AssignColor(rts)
	dot := Dot(rts)
	output(dot)
}

func cleanup() {
	palette = []string{
		"blue3",
		"aqua",
		"aquamarine4",
		"blueviolet",
		"chocolate4",
		"darkgoldenrod",
		"darkgreen",
		"darkorange",
		"deeppink",
		"green",
		"indigo",
		"midnightblue",
		"sienna4",
		"tomato1",
	}

}

func watchSave(cmd *cobra.Command, args []string, watcher *fsnotify.Watcher) {
	fmt.Println("Watching " + strings.Join(args, ", "))
	for {
		select {
		// watch for write event
		case event := <-watcher.Events:
			if event.Op == fsnotify.Write {
				cleanup()
				oneLoop(cmd, args)
			}

			// watch for errors
		case err := <-watcher.Errors:
			fmt.Println("ERROR", err)
			err = watcher.Close()
			log.Fatal(err)
		}
	}
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVarP(&outputFile, "output", "o", "", "output file")
	rootCmd.PersistentFlags().StringVarP(&format, "format", "f", "dot", "format of the output. Available formats are : dot, png, svg, jpg")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolVarP(&watch, "watch", "w", false, "watch for change in the source file. Stop with keyboard interrupt")

}

func initOutput() {
	switch format {
	case "dot":
		gFormat = graphviz.XDOT
	case "png":
		gFormat = graphviz.PNG
	case "jpg":
		gFormat = graphviz.JPG
	case "svg":
		gFormat = graphviz.SVG
	default:
		log.Println("invalid format given, defaulting to dot format")
		format = "dot"
	}

	if format != "dot" && outputFile == "" {
		log.Fatalf("Cannot output an image to stdout. Please provide an output file")
	}
}

func parse(cmd *cobra.Command, args []string) []*RelationTuple {
	var rts []*RelationTuple

	for _, fn := range args {
		rtss, err := parseFile(cmd, fn)
		if err != nil {
			log.Fatalf("%v", err)
		}
		rts = append(rts, rtss...)
	}
	return rts
}

func output(dot string) {
	g := graphviz.New()
	if outputFile == "" {
		fmt.Print(dot)
	} else {
		graph, err := graphviz.ParseBytes([]byte(dot))
		if err != nil {
			log.Fatal("unable to parse generated dot")
		}
		g.RenderFilename(graph, gFormat, outputFile)
	}

}

type ColorGroup struct {
	Color string
	Group []*RelationTuple
}

// Merges cg2 into cg1. Also updates the ColorGroupMap.
func MergeGroups(cg1 *ColorGroup, cg2 *ColorGroup, ColorGroupMap map[string]*ColorGroup) {
	if cg1 == cg2 {
		return
	}
	for _, r := range cg2.Group {
		ColorGroupMap[r.LeftColorID()] = cg1
		rID := r.RightColorID()
		if rID != "" {
			ColorGroupMap[rID] = cg1
		}
		cg1.Group = append(cg1.Group, r)
		r.Color = cg1.Color
	}
	palette = append(palette, cg2.Color)
	cg2.Color = ""
	cg2.Group = []*RelationTuple{}
}

func AssignColor(rts []*RelationTuple) {
	colorGroupMap := map[string]*ColorGroup{}
	for _, v := range rts {

		cg1, ok := colorGroupMap[v.LeftColorID()]
		if ok {
			cg1.Group = append(cg1.Group, v)
		} else {
			cg1 = NewColorGroup(PickColor())
			colorGroupMap[v.LeftColorID()] = cg1
			cg1.Group = append(cg1.Group, v)
		}
		v.Color = cg1.Color

		r := v.RightColorID()
		if r != "" {
			cg2, ok := colorGroupMap[r]
			if ok {
				MergeGroups(cg2, cg1, colorGroupMap)
			} else {
				colorGroupMap[r] = cg1
			}

		}

	}
}

func NewColorGroup(color string) *ColorGroup {
	return &ColorGroup{Color: color}
}

func PickColor() string {
	n := len(palette)
	if n == 0 {
		panic("empty palette")
	}
	res := palette[n-1]
	palette = palette[:n-1]
	return res
}

func Dot(rts []*RelationTuple) string {
	sb := strings.Builder{}
	sb.WriteString("digraph {\n")
	for _, r := range rts {
		sb.WriteString(fmt.Sprintf("  \"%v\" -> \"%v\" [ label=\"%v\", color=\"%v\"];\n", r.Namespace+":"+r.Object, r.Subject, r.Relation, r.Color))
	}
	sb.WriteString("}\n")
	return sb.String()
}

func parseFile(cmd *cobra.Command, fn string) ([]*RelationTuple, error) {
	var f io.Reader
	if fn == "-" {
		// set human readable filename here for debug and error messages
		fn = "stdin"
		f = cmd.InOrStdin()
	} else {
		ff, err := os.Open(fn)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Could not open file %s: %v\n", fn, err)
			return nil, err
		}
		defer ff.Close()
		f = ff
	}

	fc, err := io.ReadAll(f)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Could read file %s: %v\n", fn, err)
		return nil, err
	}

	parts := strings.Split(string(fc), "\n")
	rts := make([]*RelationTuple, 0, len(parts))
	for i, row := range parts {
		row = strings.TrimSpace(row)
		// ignore comments and empty lines
		if row == "" || strings.HasPrefix(row, "//") {
			continue
		}

		rt, err := (&RelationTuple{}).FromString(row)
		if err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Could not decode %s:%d\n  %s\n\n%v\n", fn, i+1, row, err)
			return nil, err
		}
		rts = append(rts, rt)
	}

	return rts, nil
}

type RelationTuple struct {
	Namespace string  `json:"namespace"`
	Object    string  `json:"object"`
	Relation  string  `json:"relation"`
	Subject   Subject `json:"subject"`
	Color     string
}

// Returns the left part of the Namespace+Object+Relation as a string for a colorID
func (r RelationTuple) LeftColorID() string {
	return r.Namespace + ":" + r.Object + "#" + r.Relation
}

// Returns the subjectSet if Subject is a set, empty string otherwise
func (r RelationTuple) RightColorID() string {
	set := r.Subject.SubjectSet()
	if set != nil {
		return set.Namespace + ":" + set.Object + "#" + set.Relation
	}
	return ""
}

type SubjectID struct {
	ID string `json:"id"`
}

type SubjectSet struct {
	// Namespace of the Subject Set
	//
	Namespace string `json:"namespace"`

	// Object of the Subject Set
	//
	// required: true
	Object string `json:"object"`

	// Relation of the Subject Set
	//
	// required: true
	Relation string `json:"relation"`
}

func (r *RelationTuple) FromString(s string) (*RelationTuple, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected input to contain ':'")
	}
	r.Namespace = parts[0]

	parts = strings.SplitN(parts[1], "#", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected input to contain '#'")
	}
	r.Object = parts[0]

	parts = strings.SplitN(parts[1], "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected input to contain '@'")
	}
	r.Relation = parts[0]

	// remove optional brackets around the subject set
	sub := strings.Trim(parts[1], "()")

	var err error
	r.Subject, err = SubjectFromString(sub)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func SubjectFromString(s string) (Subject, error) {
	if strings.Contains(s, "#") {
		return (&SubjectSet{}).FromString(s)
	}
	return (&SubjectID{}).FromString(s)
}

type Subject interface {
	String() string
	FromString(string) (Subject, error)
	Equals(interface{}) bool
	SubjectID() *string
	SubjectSet() *SubjectSet
}

var _ Subject = &SubjectID{}
var _ Subject = &SubjectSet{}

func (s *SubjectID) String() string {
	return s.ID
}

func (s *SubjectSet) String() string {
	return fmt.Sprintf("%s:%s", s.Namespace, s.Object)
}

func (s *SubjectID) FromString(str string) (Subject, error) {
	s.ID = str
	return s, nil
}

func (s *SubjectSet) FromString(str string) (Subject, error) {
	parts := strings.Split(str, "#")
	if len(parts) != 2 {
		return nil, errors.WithStack(errors.New("malformed input"))
	}

	innerParts := strings.Split(parts[0], ":")
	if len(innerParts) != 2 {
		return nil, errors.WithStack(errors.New("malformed input"))
	}

	s.Namespace = innerParts[0]
	s.Object = innerParts[1]
	s.Relation = parts[1]

	return s, nil
}

func (s *SubjectSet) SubjectID() *string {
	return nil
}

func (s *SubjectSet) SubjectSet() *SubjectSet {
	return s
}

func (s *SubjectID) SubjectID() *string {
	return &s.ID
}

func (s *SubjectID) SubjectSet() *SubjectSet {
	return nil
}

func (s *SubjectID) Equals(v interface{}) bool {
	uv, ok := v.(*SubjectID)
	if !ok {
		return false
	}
	return uv.ID == s.ID
}

func (s *SubjectSet) Equals(v interface{}) bool {
	uv, ok := v.(*SubjectSet)
	if !ok {
		return false
	}
	return uv.Relation == s.Relation && uv.Object == s.Object && uv.Namespace == s.Namespace
}
