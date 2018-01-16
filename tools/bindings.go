package tools

import (
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"unsafe"

	"gopkg.in/bblfsh/sdk.v1/uast"
)

// #cgo CXXFLAGS: -I/usr/local/include -I/usr/local/include/libxml2 -I/usr/include -I/usr/include/libxml2
// #cgo LDFLAGS: -lxml2
// #include "bindings.h"
import "C"

var findMutex sync.Mutex
var itMutex sync.Mutex
var pool cstringPool

// Traversal strategy for UAST trees
type TreeOrder int
const (
	// PreOrder traversal
	PreOrder TreeOrder = iota
	// PostOrder traversal
	PostOrder
	// LevelOrder (aka breadth-first) traversal
	LevelOrder
)

// Iterator allows for traversal over a UAST tree.
type Iterator struct {
	iterPtr C.uintptr_t
	finished bool
}

func init() {
	C.CreateUast()
}

func nodeToPtr(node *uast.Node) C.uintptr_t {
	return C.uintptr_t(uintptr(unsafe.Pointer(node)))
}

func ptrToNode(ptr C.uintptr_t) *uast.Node {
	return (*uast.Node)(unsafe.Pointer(uintptr(ptr)))
}

// Filter takes a `*uast.Node` and a xpath query and filters the tree,
// returning the list of nodes that satisfy the given query.
// Filter is thread-safe but not concurrent by an internal global lock.
func Filter(node *uast.Node, xpath string) ([]*uast.Node, error) {
	if len(xpath) == 0 {
		return nil, nil
	}

	// Find is not thread-safe bacause of the underlining C API
	findMutex.Lock()
	defer findMutex.Unlock()

	// convert go string to C string
	cquery := pool.getCstring(xpath)

	// Make sure we release the pool of strings
	defer pool.release()

	// stop GC
	gcpercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(gcpercent)

	ptr := nodeToPtr(node)
	if !C.Filter(ptr, cquery) {
		error := C.Error()
		return nil, fmt.Errorf("UastFilter() failed: %s", C.GoString(error))
		C.free(unsafe.Pointer(error))
	}

	nu := int(C.Size())
	results := make([]*uast.Node, nu)
	for i := 0; i < nu; i++ {
		results[i] = ptrToNode(C.At(C.int(i)))
	}
	return results, nil
}

//export goGetInternalType
func goGetInternalType(ptr C.uintptr_t) *C.char {
	return pool.getCstring(ptrToNode(ptr).InternalType)
}

//export goGetToken
func goGetToken(ptr C.uintptr_t) *C.char {
	return pool.getCstring(ptrToNode(ptr).Token)
}

//export goGetChildrenSize
func goGetChildrenSize(ptr C.uintptr_t) C.int {
	return C.int(len(ptrToNode(ptr).Children))
}

//export goGetChild
func goGetChild(ptr C.uintptr_t, index C.int) C.uintptr_t {
	child := ptrToNode(ptr).Children[int(index)]
	return nodeToPtr(child)
}

//export goGetRolesSize
func goGetRolesSize(ptr C.uintptr_t) C.int {
	return C.int(len(ptrToNode(ptr).Roles))
}

//export goGetRole
func goGetRole(ptr C.uintptr_t, index C.int) C.uint16_t {
	role := ptrToNode(ptr).Roles[int(index)]
	return C.uint16_t(role)
}

//export goGetPropertiesSize
func goGetPropertiesSize(ptr C.uintptr_t) C.int {
	return C.int(len(ptrToNode(ptr).Properties))
}

//export goGetPropertyKey
func goGetPropertyKey(ptr C.uintptr_t, index C.int) *C.char {
	var keys []string
	for k := range ptrToNode(ptr).Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return pool.getCstring(keys[int(index)])
}

//export goGetPropertyValue
func goGetPropertyValue(ptr C.uintptr_t, index C.int) *C.char {
	p := ptrToNode(ptr).Properties
	var keys []string
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return pool.getCstring(p[keys[int(index)]])
}

//export goHasStartOffset
func goHasStartOffset(ptr C.uintptr_t) C.bool {
	return ptrToNode(ptr).StartPosition != nil
}

//export goGetStartOffset
func goGetStartOffset(ptr C.uintptr_t) C.uint32_t {
	p := ptrToNode(ptr).StartPosition
	if p != nil {
		return C.uint32_t(p.Offset)
	}
	return 0
}

//export goHasStartLine
func goHasStartLine(ptr C.uintptr_t) C.bool {
	return ptrToNode(ptr).StartPosition != nil
}

//export goGetStartLine
func goGetStartLine(ptr C.uintptr_t) C.uint32_t {
	p := ptrToNode(ptr).StartPosition
	if p != nil {
		return C.uint32_t(p.Line)
	}
	return 0
}

//export goHasStartCol
func goHasStartCol(ptr C.uintptr_t) C.bool {
	return ptrToNode(ptr).StartPosition != nil
}

//export goGetStartCol
func goGetStartCol(ptr C.uintptr_t) C.uint32_t {
	p := ptrToNode(ptr).StartPosition
	if p != nil {
		return C.uint32_t(p.Col)
	}
	return 0
}

//export goHasEndOffset
func goHasEndOffset(ptr C.uintptr_t) C.bool {
	return ptrToNode(ptr).EndPosition != nil
}

//export goGetEndOffset
func goGetEndOffset(ptr C.uintptr_t) C.uint32_t {
	p := ptrToNode(ptr).EndPosition
	if p != nil {
		return C.uint32_t(p.Offset)
	}
	return 0
}

//export goHasEndLine
func goHasEndLine(ptr C.uintptr_t) C.bool {
	return ptrToNode(ptr).EndPosition != nil
}

//export goGetEndLine
func goGetEndLine(ptr C.uintptr_t) C.uint32_t {
	p := ptrToNode(ptr).EndPosition
	if p != nil {
		return C.uint32_t(p.Line)
	}
	return 0
}

//export goHasEndCol
func goHasEndCol(ptr C.uintptr_t) C.bool {
	return ptrToNode(ptr).EndPosition != nil
}

//export goGetEndCol
func goGetEndCol(ptr C.uintptr_t) C.uint32_t {
	p := ptrToNode(ptr).EndPosition
	if p != nil {
		return C.uint32_t(p.Col)
	}
	return 0
}

// NewIterator constructs a new Iterator starting from the given `Node` and
// iterating with the traversal strategy given by the `order` parameter. Once
// the iteration have finished or you don't need the iterator anymore you must
// dispose it with the Dispose() method (or call it with `defer`).
func NewIterator(node *uast.Node, order TreeOrder) (*Iterator, error) {
	itMutex.Lock()
	defer itMutex.Unlock()

	// stop GC
	gcpercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(gcpercent)

	ptr := nodeToPtr(node)
	it := C.IteratorNew(ptr, C.int(order))
	if it == 0 {
		error := C.Error()
		return nil, fmt.Errorf("UastIteratorNew() failed: %s", C.GoString(error))
		C.free(unsafe.Pointer(error))
	}

	return &Iterator {
		iterPtr: it,
		finished: false,
	}, nil
}

// Next retrieves the next `Node` in the tree's traversal or `nil` if there are no more
// nodes. Calling `Next()` on a finished iterator after the first `nil` will
// return an error.This is thread-safe but not concurrent by an internal global lock.
func (i *Iterator) Next() (*uast.Node, error) {
	itMutex.Lock()
	defer itMutex.Unlock()

	if i.finished {
		return nil, fmt.Errorf("Next() called on finished iterator")
	}

	// stop GC
	gcpercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(gcpercent)

	pnode := C.IteratorNext(i.iterPtr);
	if pnode == 0 {
		// End of the iteration
		i.finished = true
		return nil, nil
	}
	return ptrToNode(pnode), nil
}

// Iterate function is similar to Next() but returns the `Node`s in a channel. It's mean
// to be used with the `for node := range myIter.Iterate() {}` loop.
func (i *Iterator) Iterate() <- chan *uast.Node {
	c := make(chan *uast.Node)
	if i.finished {
		close(c)
		return c
	}

	go func() {
		for {
			n, err := i.Next()
			if n == nil || err != nil {
				close(c)
				break
			}

			c <- n
		}
	}()

	return c
}

// Dispose must be called once you've finished using the iterator or preventively
// with `defer` to free the iterator resources. Failing to do so would produce
// a memory leak.
func (i *Iterator) Dispose() {
	itMutex.Lock()
	defer itMutex.Unlock()

	if i.iterPtr != 0 {
		C.IteratorFree(i.iterPtr)
		i.iterPtr = 0
	}
	i.finished = true
}
