package tonstorage

import (
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-go/tlb"
)

func init() {
	tl.Register(TorrentInfoContainer{}, "storage.torrentInfo data:bytes = storage.TorrentInfo")
	tl.Register(GetTorrentInfo{}, "storage.getTorrentInfo = storage.TorrentInfo")
	tl.Register(Piece{}, "storage.piece proof:bytes data:bytes = storage.Piece")
	tl.Register(GetPiece{}, "storage.getPiece piece_id:int = storage.Piece")
}

type TorrentInfoContainer struct {
	Data []byte `tl:"bytes"`
}

type GetTorrentInfo struct{}

type Piece struct {
	Proof []byte `tl:"bytes"`
	Data  []byte `tl:"bytes"`
}

type GetPiece struct {
	PieceID int32 `tl:"int"`
}

type TorrentInfo struct {
	PieceSize   uint32   `tlb:"## 32"`
	FileSize    uint64   `tlb:"## 64"`
	RootHash    []byte   `tlb:"bits 256"`
	HeaderSize  uint64   `tlb:"## 64"`
	HeaderHash  []byte   `tlb:"bits 256"`
	Description tlb.Text `tlb:"."`
}
