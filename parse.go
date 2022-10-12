package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

var ignoreMnemo = map[string]bool{
	".build_version":           true,
	".section":                 true,
	".globl":                   true,
	".data_region":             true,
	".end_data_region":         true,
	".loh":                     true,
	".subsections_via_symbols": true,
}

var reLabel = regexp.MustCompile(`\b([lL](BB|JTI|CPI)\d+_\d+|_[\w.]+)(@PAGE|@PAGEOFF)?\b`)

type Prog struct {
	arch Arch
	lbls map[string]*Label
	bbs  []*BasicBlock
}

func asmParse(r io.Reader, arch Arch) (p *Prog, err error) {
	p = &Prog{
		lbls: make(map[string]*Label),
		arch: arch,
	}

	entry, err := arch.EntryBlock()
	if err != nil {
		return
	}
	p.bbs = append(p.bbs, entry)

	var lastBB *BasicBlock
	var lastLabel *Label
	sets := make(map[string]string)

	getLabel := func(name string) *Label {
		if r, ok := p.lbls[name]; ok {
			return r
		}
		r := &Label{ID: name}
		p.lbls[name] = r
		return r
	}

	ea := entry.Size()
	scan := bufio.NewScanner(r)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())

		// drop comment
		if !strings.HasPrefix(line, ".asci") {
			if idx := strings.Index(line, arch.CommentToken()); idx != -1 {
				line = line[:idx]
			}
		}
		// trim
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// label
		if line[len(line)-1] == ':' {
			// skip Lloh
			if strings.HasPrefix(line, "Lloh") {
				continue
			}
			name := line[:len(line)-1]
			// check
			if reLabel.FindString(name) != name {
				err = fmt.Errorf("invalid label: %s", name)
				return
			}
			if lastLabel != nil {
				err = fmt.Errorf("continuous label: %s", name)
				return
			}
			lastLabel = getLabel(name)

			lastBB = nil
			continue
		}
		// split
		var mnemo, opers string
		if idx := strings.IndexFunc(line, unicode.IsSpace); idx != -1 {
			mnemo, opers = line[:idx], strings.TrimSpace(line[idx+1:])
		} else {
			mnemo = line
		}
		// ignore
		if ignoreMnemo[mnemo] {
			continue
		}

		// set/long
		if mnemo == ".set" {
			idx := strings.IndexByte(opers, ',')
			sets[opers[:idx]] = strings.TrimSpace(opers[idx+1:])
			continue
		}
		if mnemo == ".long" {
			if v, ok := sets[opers]; ok {
				opers = v
			}
		}

		var los []LabelOperand
		opers = reLabel.ReplaceAllStringFunc(opers, func(old string) string {
			los = append(los, newLabelOperand(old, getLabel))
			return "%d"
		})
		var instr Instr
		if instr, err = arch.Instr(ea, mnemo, opers, los); err != nil {
			return
		}
		ea += instr.Size()

		if lastBB == nil {
			lastBB = &BasicBlock{}
			p.bbs = append(p.bbs, lastBB)
			if lastLabel != nil {
				lastLabel.Bind(lastBB)
				lastLabel = nil
			}
		}

		lastBB.Instrs = append(lastBB.Instrs, instr)

		if instr.Kind() == InstrKind_Jmp || instr.Kind() == InstrKind_Cond_Jmp || instr.Kind() == InstrKind_Ret {
			lastBB = nil
		}
	}
	if err = scan.Err(); err != nil {
		return
	}

	// check label
	for k, v := range p.lbls {
		if v.BB == nil {
			err = fmt.Errorf("nil label: %s", k)
			return
		}
	}

	// fill succs
	for i, v := range p.bbs {
		switch ins := v.Last(); ins.Kind() {
		case InstrKind_Ret:
		case InstrKind_Jmp, InstrKind_Cond_Jmp:
			if o := ins.LabelOperand(); o != nil {
				v.Succs = append(v.Succs, o.BB())
			}
			if ins.Kind() == InstrKind_Jmp {
				continue
			}
			fallthrough
		default:
			if i < len(p.bbs)-1 {
				v.Succs = append(v.Succs, p.bbs[i+1])
			}
		}
	}

	return
}

func isData(mnemo string) bool {
	switch mnemo {
	case ".quad", ".long", ".short", ".byte", ".space", ".ascii", ".asciz":
		return true
	}
	return false
}
