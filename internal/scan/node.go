// Package scan walks a directory tree in parallel and builds a size tree
// based on the space actually allocated on disk (st_blocks * 512).
package scan

import (
	"container/heap"
	"sort"
	"strings"
)

// Node is one entry (file, directory or symlink) in the scanned tree.
type Node struct {
	Name      string
	Path      string
	Parent    *Node
	IsDir     bool
	IsSymlink bool
	Size      int64 // allocated bytes; for directories the recursive total
	Items     int64 // number of descendants (0 for non-directories)
	Children  []*Node
	Err       string // non-empty if the directory could not be read
	OtherFS   bool   // directory on another filesystem, not descended into
	Linked    bool   // extra hard link to data already counted elsewhere
}

// Remove detaches n from its parent and fixes the totals of all ancestors.
// It is used after an entry has been moved to the trash.
func (n *Node) Remove() {
	p := n.Parent
	if p == nil {
		return
	}
	for i, c := range p.Children {
		if c == n {
			p.Children = append(p.Children[:i], p.Children[i+1:]...)
			break
		}
	}
	for a := p; a != nil; a = a.Parent {
		a.Size -= n.Size
		a.Items -= n.Items + 1
	}
	n.Parent = nil
}

// SortMode selects how children are ordered.
type SortMode int

const (
	SortSize SortMode = iota
	SortName
	SortItems
	numSortModes
)

func (m SortMode) String() string {
	switch m {
	case SortName:
		return "名稱"
	case SortItems:
		return "項目數"
	}
	return "大小"
}

// Next cycles size -> name -> items -> size.
func (m SortMode) Next() SortMode { return (m + 1) % numSortModes }

// SortNodes sorts in place: size and items descending, name ascending
// (case-insensitive). Ties fall back to name so the order is stable.
func SortNodes(ns []*Node, mode SortMode) {
	sort.SliceStable(ns, func(i, j int) bool {
		a, b := ns[i], ns[j]
		switch mode {
		case SortName:
			return lessName(a, b)
		case SortItems:
			if a.Items != b.Items {
				return a.Items > b.Items
			}
		default:
			if a.Size != b.Size {
				return a.Size > b.Size
			}
		}
		return lessName(a, b)
	})
}

func lessName(a, b *Node) bool {
	x, y := strings.ToLower(a.Name), strings.ToLower(b.Name)
	if x != y {
		return x < y
	}
	return a.Name < b.Name
}

// Filter returns the nodes whose name contains q (case-insensitive) and whose
// size is at least minSize. An empty q matches everything.
func Filter(ns []*Node, q string, minSize int64) []*Node {
	q = strings.ToLower(q)
	out := make([]*Node, 0, len(ns))
	for _, n := range ns {
		if n.Size < minSize {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(n.Name), q) {
			continue
		}
		out = append(out, n)
	}
	return out
}

type minHeap []*Node

func (h minHeap) Len() int            { return len(h) }
func (h minHeap) Less(i, j int) bool  { return h[i].Size < h[j].Size }
func (h minHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x interface{}) { *h = append(*h, x.(*Node)) }
func (h *minHeap) Pop() interface{} {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}

// TopFiles returns the n largest non-directory entries under root
// (largest first) whose size is at least minSize.
func TopFiles(root *Node, n int, minSize int64) []*Node {
	if n <= 0 || root == nil {
		return nil
	}
	h := &minHeap{}
	var walk func(*Node)
	walk = func(d *Node) {
		for _, c := range d.Children {
			if c.IsDir {
				walk(c)
				continue
			}
			if c.Size < minSize || c.Size == 0 {
				continue
			}
			if h.Len() < n {
				heap.Push(h, c)
			} else if c.Size > (*h)[0].Size {
				(*h)[0] = c
				heap.Fix(h, 0)
			}
		}
	}
	if root.IsDir {
		walk(root)
	} else if root.Size >= minSize {
		heap.Push(h, root)
	}
	out := make([]*Node, h.Len())
	copy(out, *h)
	SortNodes(out, SortSize)
	return out
}
