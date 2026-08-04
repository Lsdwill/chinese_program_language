package ast

import "huayan/internal/source"

type Node interface {
	node()
	GetSpan() source.Span
}
type Stmt interface {
	Node
	stmt()
}
type Expr interface {
	Node
	expr()
}

type Program struct {
	Module     string
	Statements []Stmt
	Span       source.Span
}

func (p *Program) node()                {}
func (p *Program) GetSpan() source.Span { return p.Span }

type ImportStmt struct {
	Path          string
	Alias         string
	Names         []string
	Span          source.Span
	ResolvedSlots map[string]int
}

func (*ImportStmt) node()                  {}
func (*ImportStmt) stmt()                  {}
func (s *ImportStmt) GetSpan() source.Span { return s.Span }

type VarDecl struct {
	Name          string
	Value         Expr
	Constant      bool
	Public        bool
	Span          source.Span
	ResolvedSlot  int
	ResolvedLocal bool
}

func (*VarDecl) node()                  {}
func (*VarDecl) stmt()                  {}
func (s *VarDecl) GetSpan() source.Span { return s.Span }

type FunctionDecl struct {
	Name          string
	Params        []string
	Body          []Stmt
	Public        bool
	Span          source.Span
	LocalSlots    int
	ResolvedSlot  int
	ResolvedLocal bool
}

func (*FunctionDecl) node()                  {}
func (*FunctionDecl) stmt()                  {}
func (s *FunctionDecl) GetSpan() source.Span { return s.Span }

type ExprStmt struct {
	Expression Expr
	Span       source.Span
}

func (*ExprStmt) node()                  {}
func (*ExprStmt) stmt()                  {}
func (s *ExprStmt) GetSpan() source.Span { return s.Span }

type IfBranch struct {
	Condition Expr
	Body      []Stmt
	Span      source.Span
}
type IfStmt struct {
	Branches []IfBranch
	Else     []Stmt
	Span     source.Span
}

func (*IfStmt) node()                  {}
func (*IfStmt) stmt()                  {}
func (s *IfStmt) GetSpan() source.Span { return s.Span }

type WhileStmt struct {
	Condition Expr
	Body      []Stmt
	Span      source.Span
}

func (*WhileStmt) node()                  {}
func (*WhileStmt) stmt()                  {}
func (s *WhileStmt) GetSpan() source.Span { return s.Span }

type ForStmt struct {
	Name          string
	Iterable      Expr
	Body          []Stmt
	Span          source.Span
	ResolvedSlot  int
	ResolvedLocal bool
}

func (*ForStmt) node()                  {}
func (*ForStmt) stmt()                  {}
func (s *ForStmt) GetSpan() source.Span { return s.Span }

type TryStmt struct {
	Body          []Stmt
	CatchName     string
	CatchBody     []Stmt
	Finally       []Stmt
	HasFinally    bool
	Span          source.Span
	ResolvedSlot  int
	ResolvedLocal bool
}

func (*TryStmt) node()                  {}
func (*TryStmt) stmt()                  {}
func (s *TryStmt) GetSpan() source.Span { return s.Span }

type ReturnStmt struct {
	Value Expr
	Span  source.Span
}

func (*ReturnStmt) node()                  {}
func (*ReturnStmt) stmt()                  {}
func (s *ReturnStmt) GetSpan() source.Span { return s.Span }

type BreakStmt struct{ Span source.Span }

func (*BreakStmt) node()                  {}
func (*BreakStmt) stmt()                  {}
func (s *BreakStmt) GetSpan() source.Span { return s.Span }

type ContinueStmt struct{ Span source.Span }

func (*ContinueStmt) node()                  {}
func (*ContinueStmt) stmt()                  {}
func (s *ContinueStmt) GetSpan() source.Span { return s.Span }

type ThrowStmt struct {
	Value Expr
	Span  source.Span
}

func (*ThrowStmt) node()                  {}
func (*ThrowStmt) stmt()                  {}
func (s *ThrowStmt) GetSpan() source.Span { return s.Span }

type Literal struct {
	Kind  string
	Value any
	Span  source.Span
}

func (*Literal) node()                  {}
func (*Literal) expr()                  {}
func (e *Literal) GetSpan() source.Span { return e.Span }

type Name struct {
	Value        string
	Span         source.Span
	ResolvedSlot int
	ResolvedKind NameResolutionKind
}

type NameResolutionKind uint8

const (
	NameUnresolved NameResolutionKind = iota
	NameGlobal
	NameLocal
	NameUpvalue
)

func (*Name) node()                  {}
func (*Name) expr()                  {}
func (e *Name) GetSpan() source.Span { return e.Span }

type Unary struct {
	Operator string
	Right    Expr
	Span     source.Span
}

func (*Unary) node()                  {}
func (*Unary) expr()                  {}
func (e *Unary) GetSpan() source.Span { return e.Span }

type Binary struct {
	Left     Expr
	Operator string
	Right    Expr
	Span     source.Span
}

func (*Binary) node()                  {}
func (*Binary) expr()                  {}
func (e *Binary) GetSpan() source.Span { return e.Span }

type Assign struct {
	Target Expr
	Value  Expr
	Span   source.Span
}

func (*Assign) node()                  {}
func (*Assign) expr()                  {}
func (e *Assign) GetSpan() source.Span { return e.Span }

type Call struct {
	Callee Expr
	Args   []Expr
	Span   source.Span
}

func (*Call) node()                  {}
func (*Call) expr()                  {}
func (e *Call) GetSpan() source.Span { return e.Span }

type Index struct {
	Object Expr
	Key    Expr
	Span   source.Span
}

func (*Index) node()                  {}
func (*Index) expr()                  {}
func (e *Index) GetSpan() source.Span { return e.Span }

type Field struct {
	Object Expr
	Name   string
	Span   source.Span
}

func (*Field) node()                  {}
func (*Field) expr()                  {}
func (e *Field) GetSpan() source.Span { return e.Span }

type List struct {
	Items []Expr
	Span  source.Span
}

func (*List) node()                  {}
func (*List) expr()                  {}
func (e *List) GetSpan() source.Span { return e.Span }

type Pair struct {
	Key   Expr
	Value Expr
}
type Dict struct {
	Pairs  []Pair
	Record bool
	Span   source.Span
}

func (*Dict) node()                  {}
func (*Dict) expr()                  {}
func (e *Dict) GetSpan() source.Span { return e.Span }
