package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"sort"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
)

var yamlsortUsage = `
yaml sorter. read yaml text from stdin or file, output map key sorted text to stdout or file.
`

type yamlsortCmd struct {
	stdin            io.Reader
	stdout           io.Writer
	stderr           io.Writer
	inputfilename    string
	outputfilename   string
	blnNormalMarshal bool
	blnJSONMarshal   bool
	blnQuoteString   bool
}

func newRootCmd(args []string) *cobra.Command {
	yamlsort := &yamlsortCmd{}

	cmd := &cobra.Command{
		Use:   "yamlsort",
		Short: "yaml sorter",
		Long:  yamlsortUsage,
		RunE: func(c *cobra.Command, args []string) error {
			return yamlsort.run()
		},
	}

	f := cmd.Flags()
	f.StringVarP(&yamlsort.inputfilename, "input-file", "i", "", "path to input file name")
	f.StringVarP(&yamlsort.outputfilename, "output-file", "o", "", "path to output file name")
	f.BoolVar(&yamlsort.blnQuoteString, "quote-string", false, "string value is always quoted in output")
	f.BoolVar(&yamlsort.blnNormalMarshal, "normal", false, "use marshal (github.com/ghodss/yaml)")
	f.BoolVar(&yamlsort.blnJSONMarshal, "json", false, "use json marshal (encoding/json)")

	yamlsort.stdin = os.Stdin
	yamlsort.stdout = os.Stdout
	yamlsort.stderr = os.Stderr

	return cmd
}

func main() {
	cmd := newRootCmd(os.Args[1:])
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func (c *yamlsortCmd) run() error {

	myReadBytes := []byte{}
	var err error

	// check input-file option
	if len(c.inputfilename) > 0 {
		// read from file
		myReadBytes, err = ioutil.ReadFile(c.inputfilename)
		if err != nil {
			return err
		}
	} else {
		// read from stdin
		myReadBuffer := new(bytes.Buffer)
		_, err := io.Copy(myReadBuffer, c.stdin)
		if err != nil {
			return err
		}
		myReadBytes = myReadBuffer.Bytes()
	}

	// check output-file option
	outputWriter := c.stdout
	var flushWriter *bufio.Writer
	if len(c.outputfilename) > 0 {
		ofp, err := os.Create(c.outputfilename)
		if err != nil {
			return err
		}
		defer ofp.Close()
		flushWriter = bufio.NewWriter(ofp)
		outputWriter = flushWriter
	}

	// setup scanner
	reader := bytes.NewReader(myReadBytes)
	scanner := bufio.NewScanner(reader)
	onefilebuffer := new(bytes.Buffer)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if onefilebuffer.Len() > 0 {
				// marshal one file
				err = c.procOneFile(outputWriter, onefilebuffer.Bytes())
				if err != nil {
					return err
				}
				if flushWriter != nil {
					err := flushWriter.Flush()
					if err != nil {
						return err
					}
				}
				onefilebuffer = new(bytes.Buffer)
			}
		} else {
			fmt.Fprintln(onefilebuffer, line)
		}
	}
	if onefilebuffer.Len() > 0 {
		// marshal one file
		err = c.procOneFile(outputWriter, onefilebuffer.Bytes())
		if err != nil {
			return err
		}
		onefilebuffer = new(bytes.Buffer)
		if flushWriter != nil {
			err := flushWriter.Flush()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *yamlsortCmd) procOneFile(outputWriter io.Writer, inputbytes []byte) error {
	// parse yaml data
	var data interface{}
	err := yaml.Unmarshal(inputbytes, &data)
	if err != nil {
		fmt.Fprintln(c.stderr, "Unmarshal error:", err)
		return err
	}

	if c.blnNormalMarshal {
		// write yaml data with normal marshal
		outputBytes, err := yaml.Marshal(data)
		if err != nil {
			fmt.Fprintln(c.stderr, "Marshal error:", err)
			return err
		}
		fmt.Fprintln(outputWriter, "---")
		fmt.Fprintln(outputWriter, "# Marshal output")
		fmt.Fprintln(outputWriter, string(outputBytes))
	} else if c.blnJSONMarshal {
		// write json data with normal marshal
		outputBytes, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Fprintln(c.stderr, "Marshal error:", err)
			return err
		}
		fmt.Fprintln(outputWriter, "---")
		fmt.Fprintln(outputWriter, "# Marshal output")
		fmt.Fprintln(outputWriter, string(outputBytes))

	} else {
		// write my marshal
		outputBytes2, err := c.myMarshal(data)
		if err != nil {
			fmt.Fprintln(c.stderr, "myMarshal error:", err)
			return err
		}
		fmt.Fprintln(outputWriter, "---")
		fmt.Fprintln(outputWriter, "# myMarshal output")
		fmt.Fprintln(outputWriter, string(outputBytes2))
	}

	return nil
}

func (c *yamlsortCmd) myMarshal(data interface{}) ([]byte, error) {
	// create buffer
	writer := new(bytes.Buffer)
	err := c.myMershalRecursive(writer, 0, false, data)
	return writer.Bytes(), err
}

func (c *yamlsortCmd) myMershalRecursive(writer io.Writer, level int, blnParentSlide bool, data interface{}) error {
	if data == nil {
		fmt.Fprintln(writer, "")
		return nil
	}
	if m, ok := data.(map[string]interface{}); ok {
		// data is map
		// get key list
		var keylist []string
		for k := range m {
			keylist = append(keylist, k)
		}
		// sort map key, but key "name" is first
		sort.Slice(keylist, func(idx1, idx2 int) bool {
			if keylist[idx1] == "name" && keylist[idx2] == "name" {
				return false
			} else if keylist[idx1] == "name" {
				return true
			} else if keylist[idx2] == "name" {
				return false
			}
			return keylist[idx1] < keylist[idx2]
		})
		// recursive call
		for i, k := range keylist {
			v := m[k]
			indentstr := c.indentstr(level)
			// when parent element is slice and print first key value, no need to indent
			if blnParentSlide && i == 0 {
				indentstr = ""
			}
			if v == nil {
				// child is nil. print key only.
				fmt.Fprintf(writer, "%s%s:", indentstr, k)
			} else if _, ok := v.(map[string]interface{}); ok {
				// child is map
				fmt.Fprintf(writer, "%s%s:\n", indentstr, k)
			} else if _, ok := v.([]interface{}); ok {
				// child is slice
				fmt.Fprintf(writer, "%s%s:\n", indentstr, k)
			} else {
				// child is normal string
				fmt.Fprintf(writer, "%s%s: ", indentstr, k)
			}
			err := c.myMershalRecursive(writer, level+2, false, v)
			if err != nil {
				return err
			}
		}
		return nil
	} else if a, ok := data.([]interface{}); ok {
		// data is slice
		for _, v := range a {
			fmt.Fprintf(writer, "%s- ", c.indentstr(level-2))
			err := c.myMershalRecursive(writer, level, true, v)
			if err != nil {
				return err
			}
		}
		return nil
	} else if s, ok := data.(string); ok {
		// data is string
		if c.blnQuoteString {
			// string is always quoted
			fmt.Fprintf(writer, "\"%s\"\n", s)
		} else {
			fmt.Fprintln(writer, s)
		}
	} else if i, ok := data.(int); ok {
		// data is string
		fmt.Fprintln(writer, i)
	} else if f64, ok := data.(float64); ok {
		// data is string
		fmt.Fprintln(writer, f64)
	} else {
		return fmt.Errorf("unknown type:%v  data:%v", reflect.TypeOf(data), data)
	}
	return nil
}

func (c *yamlsortCmd) indentstr(level int) string {
	result := ""
	for i := 0; i < level; i++ {
		result = result + " "
	}
	return result
}