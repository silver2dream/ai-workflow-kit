package guard

import (
	"fmt"
	"strings"
)

// Prepared is the context of a prompt once it went through the input checks.
type Prepared struct {
	// Pieces are the pieces as they go into the prompt: sanitized, with an id
	// each.
	Pieces []Piece
	// Verdicts are what the checks said, the passes left out.
	Verdicts []Verdict
}

// Blocked reports whether a check blocked the prompt.
func (p *Prepared) Blocked() bool { return Blocks(p.Verdicts) }

// Prepare runs the input checks over the pieces of a prompt, in order: every
// piece must say where it came from and how far it is trusted (a piece that
// does not is a fault of the caller, and blocks); every piece is sanitized;
// the untrusted ones are screened for injection. Pieces get the ids c1, c2,
// … in order.
func Prepare(pieces []Piece) *Prepared {
	out := &Prepared{}
	for i, p := range pieces {
		p.ID = fmt.Sprintf("c%d", i+1)
		if p.Source == "" || !KnownTrust(p.Trust) {
			out.Verdicts = append(out.Verdicts, Verdict{
				Result: Block, Class: Security, Stage: StageInput,
				Rule: "provenance", Piece: p.ID, Source: p.Source,
				Reason: fmt.Sprintf("a piece of context without a source or a known trust level (%q): it is not given to an agent", p.Trust),
			})
			continue
		}
		max := MaxPieceChars
		if p.Max > 0 {
			max = p.Max
		}
		text, changes := Sanitize(p.Text, max)
		if len(changes) > 0 {
			result, class := Flag, Info
			for _, change := range changes {
				// Characters no one sees, in text an agent wrote or a stranger
				// did, are how a hidden instruction is smuggled in.
				if strings.Contains(change, "hidden character") && Untrusted(p.Trust) {
					class = Security
				}
			}
			out.Verdicts = append(out.Verdicts, Verdict{
				Result: result, Class: class, Stage: StageInput,
				Rule: "sanitize", Piece: p.ID, Source: p.Source,
				Reason: strings.Join(changes, "; "),
			})
		}
		p.Text = text
		out.Verdicts = append(out.Verdicts, Screen(p)...)
		out.Pieces = append(out.Pieces, p)
	}
	return out
}

// ForPiece returns the verdicts about one piece.
func (p *Prepared) ForPiece(id string) []Verdict {
	var out []Verdict
	for _, v := range p.Verdicts {
		if v.Piece == id {
			out = append(out, v)
		}
	}
	return out
}

// SuspiciousVerdicts turns the pieces an agent reported as suspicious into
// verdicts, for the record: an agent that saw an instruction in a data block
// says so, and a person may want to look.
func (p *Prepared) SuspiciousVerdicts(ids []string) []Verdict {
	var out []Verdict
	for _, id := range ids {
		v := Verdict{
			Result: Flag, Class: Security, Stage: StageOutput,
			Rule: "agent-reported", Piece: id,
			Reason: "the agent reported that this piece asked it to do something its instructions do not allow",
		}
		for _, piece := range p.Pieces {
			if piece.ID == id {
				v.Source = piece.Source
			}
		}
		out = append(out, v)
	}
	return out
}
