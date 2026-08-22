package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"torrent/core"
)

func WritePieceToFile(torrent *core.TorrentMetaData, pieceIdx int, data []byte, dest string) bool {
    // data can be a full piece OR a single block
    
    // Get file ranges for this piece
    ranges := GetFileFromPiece(torrent, pieceIdx, data)
    if len(ranges) == 0 {
        fmt.Printf("No files found for piece %d\n", pieceIdx)
        return false
    }

    for _, fileRange := range ranges {
        // Build full path
        fullPath := filepath.Join(dest, torrent.Info.Name, filepath.Join(fileRange.File.FilePath...))
        
        // Make sure parent directories exist
        parentDir := filepath.Dir(fullPath)
        if err := os.MkdirAll(parentDir, 0755); err != nil {
            fmt.Printf("err creating directory %s: %s\n", parentDir, err)
            return false
        }

        // Open file (create if doesn't exist, don't truncate)
        f, err := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, 0644)
        if err != nil {
            fmt.Printf("err opening file %s: %s\n", fullPath, err)
            return false
        }
        defer f.Close()

        
        _, err = f.Seek(fileRange.FileOffset, 0)
        if err != nil {
            fmt.Printf("err seeking in file: %s\n", err)
            return false
        }
        start := fileRange.PieceOffset
        end := fileRange.PieceOffset + fileRange.Length
        
       
        if end > int64(len(data)) {
            end = int64(len(data))
        }
        
        _, err = f.Write(data[start:end])
        if err != nil {
            fmt.Printf("err writing to file: %s\n", err)
            return false
        }
        
        fmt.Printf("wrote %d bytes to %s at offset %d\n", 
            fileRange.Length, filepath.Base(fullPath), fileRange.FileOffset)
    }
    
    return true
}

type FileWriteRange struct {
    File       core.FileItem
    FileOffset int64
    PieceOffset int64
    Length     int64
}

func GetFileFromPiece(torrent *core.TorrentMetaData, pieceIdx int, data []byte) []FileWriteRange{

	pieceStart := int64(pieceIdx) * torrent.Info.Piece_length
	pieceEnd := pieceStart + int64(len(data))

	var ranges []FileWriteRange

	for _, file := range torrent.Info.Files{
		fileStart := file.Offset
		fileEnd := file.Offset + file.Length

		if pieceStart < fileEnd && pieceEnd > fileStart{
			overlapStart := max(pieceStart, fileStart)
            overlapEnd := min(pieceEnd, fileEnd)

			ranges = append(ranges, 
			FileWriteRange{
				File: file,
				FileOffset: overlapStart - fileStart,
				PieceOffset: overlapStart - pieceStart,
				Length: overlapEnd - overlapStart,
			})
			
		}
	}
	return ranges
}


// reads a full piece from the actual files for verification
func ReadPieceFromFiles(torrent *core.TorrentMetaData, pieceIdx int, downloadPath string) []byte {
    pieceStart := int64(pieceIdx) * torrent.Info.Piece_length
    pieceLength := torrent.Info.Piece_length
    pieceData := make([]byte, pieceLength)
    
    for _, file := range torrent.Info.Files {
        fileStart := file.Offset
        fileEnd := file.Offset + file.Length
        
        overlapStart := max(pieceStart, fileStart)
        overlapEnd := min(pieceStart+pieceLength, fileEnd)
        
        if overlapStart < overlapEnd {
            offsetInFile := overlapStart - fileStart
            bytesToRead := overlapEnd - overlapStart
            offsetInPiece := overlapStart - pieceStart
            
            fullPath := filepath.Join(downloadPath, torrent.Info.Name, filepath.Join(file.FilePath...))
            
            f, err := os.OpenFile(fullPath, os.O_RDWR, 0644)
            if err != nil {
                fmt.Printf("Error opening file %s: %s\n", fullPath, err)
                return []byte{}
            }
            
            _, err = f.Seek(offsetInFile, 0)
            if err != nil {
                f.Close()
                fmt.Printf("Error seeking in file: %s\n", err)
                return []byte{}
            }
            
            n, err := f.Read(pieceData[offsetInPiece:offsetInPiece+bytesToRead])
            f.Close()
            
            if err != nil || int64(n) != bytesToRead {
                fmt.Printf("Error reading piece data: read %d of %d bytes\n", n, bytesToRead)
                return []byte{}
            }
        }
    }
    
    return pieceData
}

// clears a piece from files (for redownload)
func ClearPieceFromFiles(torrent *core.TorrentMetaData, pieceIdx int, downloadPath string) {
    pieceStart := int64(pieceIdx) * torrent.Info.Piece_length
    pieceLength := torrent.Info.Piece_length
    
    for _, file := range torrent.Info.Files {
        fileStart := file.Offset
        fileEnd := file.Offset + file.Length
        
        overlapStart := max(pieceStart, fileStart)
        overlapEnd := min(pieceStart+pieceLength, fileEnd)
        
        if overlapStart < overlapEnd {
            offsetInFile := overlapStart - fileStart
            bytesToClear := overlapEnd - overlapStart
            
            fullPath := filepath.Join(downloadPath, torrent.Info.Name, filepath.Join(file.FilePath...))
            
            f, err := os.OpenFile(fullPath, os.O_RDWR, 0644)
            if err != nil {
                fmt.Printf("Error opening file for clearing: %s\n", err)
                continue
            }

            zeros := make([]byte, bytesToClear)
            _, err = f.Seek(offsetInFile, 0)
            if err != nil {
                f.Close()
                fmt.Printf("Error seeking for clearing: %s\n", err)
                continue
            }
            
            _, err = f.Write(zeros)
            f.Close()
            
            if err != nil {
                fmt.Printf("error clearing data: %s\n", err)
            } else {
                fmt.Printf("cleared %d bytes from %s at offset %d\n", 
                    bytesToClear, filepath.Base(fullPath), offsetInFile)
            }
        }
    }
}