package buffer

import (
	"testing"
	"github.com/bethropolis/tide/internal/types"
)

func TestPieceTable_Insert(t *testing.T) {
	pt := NewPieceTable()
	
	// Test empty insert
	pt.Insert(types.Position{Line: 0, Col: 0}, []byte("hello"))
	if string(pt.Bytes()) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(pt.Bytes()))
	}
	
	// Test append
	pt.Insert(types.Position{Line: 0, Col: 5}, []byte(" world"))
	if string(pt.Bytes()) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(pt.Bytes()))
	}
	
	// Test prepend
	pt.Insert(types.Position{Line: 0, Col: 0}, []byte("say "))
	if string(pt.Bytes()) != "say hello world" {
		t.Errorf("Expected 'say hello world', got '%s'", string(pt.Bytes()))
	}
	
	// Test insert middle
	pt.Insert(types.Position{Line: 0, Col: 10}, []byte("beautiful "))
	if string(pt.Bytes()) != "say hello beautiful world" {
		t.Errorf("Expected 'say hello beautiful world', got '%s'", string(pt.Bytes()))
	}
}

func TestPieceTable_Delete(t *testing.T) {
	pt := NewPieceTable()
	
	pt.Insert(types.Position{Line: 0, Col: 0}, []byte("hello beautiful world"))
	
	// Delete middle
	pt.Delete(types.Position{Line: 0, Col: 6}, types.Position{Line: 0, Col: 16})
	if string(pt.Bytes()) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(pt.Bytes()))
	}
	
	// Delete start
	pt.Delete(types.Position{Line: 0, Col: 0}, types.Position{Line: 0, Col: 6})
	if string(pt.Bytes()) != "world" {
		t.Errorf("Expected 'world', got '%s'", string(pt.Bytes()))
	}
}
