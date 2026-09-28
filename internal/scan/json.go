package scan

import (
	"github.com/useless-husband/macdust/internal/humanize"
)

// JSONNode is the serialisable form of a Node.
type JSONNode struct {
	Name      string     `json:"name"`
	Path      string     `json:"path"`
	Type      string     `json:"type"` // dir, file or symlink
	Size      int64      `json:"size"`
	SizeHuman string     `json:"size_human"`
	Items     int64      `json:"items,omitempty"`
	Error     string     `json:"error,omitempty"`
	OtherFS   bool       `json:"other_filesystem,omitempty"`
	Children  []JSONNode `json:"children,omitempty"`
}

// ToJSON converts n into a JSONNode, descending at most depth levels below n
// and leaving out children smaller than minSize.
func ToJSON(n *Node, depth int, minSize int64) JSONNode {
	j := JSONNode{Name: n.Name, Path: n.Path, Type: "file", Size: n.Size,
		SizeHuman: humanize.Bytes(n.Size), Items: n.Items, Error: n.Err, OtherFS: n.OtherFS}
	if n.IsDir {
		j.Type = "dir"
	} else if n.IsSymlink {
		j.Type = "symlink"
	}
	if depth > 0 {
		for _, c := range n.Children {
			if c.Size >= minSize {
				j.Children = append(j.Children, ToJSON(c, depth-1, minSize))
			}
		}
	}
	return j
}
