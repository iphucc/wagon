// Copyright 2019 The go-interpreter Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compile

import (
	ops "github.com/go-interpreter/wagon/wasm/operators"
)

type scanner struct {
	supportedOpcodes map[byte]bool
}

// InstructionMetadata describes a bytecode instruction.
type InstructionMetadata struct {
	Op byte
	// Start represents the byte offset of this instruction
	// in the function's instruction stream.
	Start int
	// Size is the number of bytes in the instruction stream
	// needed to represent this instruction.
	Size int
}

// CompilationCandidate describes a range of bytecode that can
// be translated to native code.
type CompilationCandidate struct {
	Start            uint    // Bytecode index of the first opcode.
	End              uint    // Bytecode index of the last byte in the instruction.
	StartInstruction int     // InstructionMeta index of the first instruction.
	EndInstruction   int     // InstructionMeta index of the last instruction.
	Metrics          Metrics // Metrics about the instructions between first & last index.
}

func (s *CompilationCandidate) reset() {
	s.Start = 0
	s.End = 0
	s.StartInstruction = 0
	s.EndInstruction = 1
	s.Metrics = Metrics{}
}

// Bounds returns the beginning & end index in the bytecode which
// this candidate would replace. The end index is not inclusive.
func (s *CompilationCandidate) Bounds() (uint, uint) {
	return s.Start, s.End
}

// Metrics describes the heuristics of an instruction sequence.
type Metrics struct {
	MemoryReads, MemoryWrites uint
	StackReads, StackWrites   uint

	AllOps     int
	IntegerOps int
	FloatOps   int
}

// ScanFunc scans the given function information, emitting selections of
// bytecode which could be compiled into function code.
func (s *scanner) ScanFunc(bytecode []byte, meta *BytecodeMetadata) ([]CompilationCandidate, error) {
	var finishedCandidates []CompilationCandidate
	inProgress := CompilationCandidate{}

	for i, inst := range meta.Instructions {
		// Except for the first instruction, we cant emit a native section
		// where other parts of code try and call into us halfway. Maybe we
		// can support that in the future.
		_, hasInboundTarget := meta.InboundTargets[int64(inst.Start)]
		isInsideBranchTarget := hasInboundTarget && inst.Start > 0 && inProgress.Metrics.AllOps > 0

		if !s.supportedOpcodes[inst.Op] || isInsideBranchTarget {
			// See if the candidate can be emitted.
			if inProgress.Metrics.AllOps > 2 {
				finishedCandidates = append(finishedCandidates, inProgress)
			}
			inProgress.reset()
			continue
		}

		// Still a supported run.

		if inProgress.Metrics.AllOps == 0 {
			// First instruction of the candidate - setup structure.
			inProgress.Start = uint(inst.Start)
			inProgress.StartInstruction = i
		}
		inProgress.EndInstruction = i + 1
		inProgress.End = uint(inst.Start) + uint(inst.Size)

		// TODO: Add to this table as backends support more opcodes.
		switch inst.Op {
		case ops.I64Const, ops.GetLocal:
			inProgress.Metrics.IntegerOps++
			inProgress.Metrics.StackWrites++
		case ops.SetLocal:
			inProgress.Metrics.IntegerOps++
			inProgress.Metrics.StackReads++
		case ops.I64Eqz:
			inProgress.Metrics.IntegerOps++
			inProgress.Metrics.StackReads++
			inProgress.Metrics.StackWrites++

		case ops.I64Eq, ops.I64Ne, ops.I64LtU, ops.I64GtU, ops.I64LeU, ops.I64GeU,
			ops.I64Shl, ops.I64ShrU, ops.I64ShrS,
			ops.I64DivU, ops.I64RemU, ops.I64DivS, ops.I64RemS,
			ops.I64Add, ops.I64Sub, ops.I64Mul, ops.I64And, ops.I64Or, ops.I64Xor:
			inProgress.Metrics.IntegerOps++
			inProgress.Metrics.StackReads += 2
			inProgress.Metrics.StackWrites++
		}
		inProgress.Metrics.AllOps++
	}

	// End of instructions - emit the inProgress candidate if
	// its at least 3 instructions.
	if inProgress.Metrics.AllOps > 2 {
		finishedCandidates = append(finishedCandidates, inProgress)
	}
	return finishedCandidates, nil
}
