package core



type TorrentMetaData struct {
	Announce string 
	Announce_list []string
	Info *Info
}

type Info struct{
	Piece_length int64
	Pieces string
	Name string 
	Length int64
	Files []FileItem
}

type FileItem struct{
	Length int64
	FilePath []string
}
