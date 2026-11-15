// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import "time"

// Archive states.
const (
	ArchiveReady   = "ready"
	ArchiveFailed  = "failed"
	ArchivePending = "pending"
)

// DocumentArchive records a saved reader view archive of a document. The
// archive files live in a directory named after the ID under the data directory.
type DocumentArchive struct {
	ID        uint      `gorm:"primaryKey"                        json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `gorm:"index:idx_archive_url_user;not null" json:"url"`
	UserID    uint      `gorm:"index:idx_archive_url_user"          json:"user_id"`
	Title     string    `json:"title"`
	Engine    string    `json:"engine"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	Size      int64     `json:"size"`
	Images    bool      `json:"images"`
	Mobile    bool      `json:"mobile"`
	// VideoStatus is empty when no video was requested.
	VideoStatus string `json:"video_status,omitempty"`
	VideoFile   string `json:"video_file,omitempty"`
	// Dir is the directory holding the archive files.
	Dir string `json:"-"`
}

// CreateArchive stores a new archive record.
func CreateArchive(a *DocumentArchive) error {
	return DB.Create(a).Error
}

// SaveArchive updates an archive record.
func SaveArchive(a *DocumentArchive) error {
	return DB.Save(a).Error
}

// GetArchive returns one archive owned by userID.
func GetArchive(id, userID uint) (*DocumentArchive, error) {
	var a DocumentArchive
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// GetArchives returns the archives of a document, newest first.
func GetArchives(url string, userID uint) ([]*DocumentArchive, error) {
	var archives []*DocumentArchive
	err := DB.Where("url = ? AND user_id = ?", url, userID).Order("created_at DESC, id DESC").Find(&archives).Error
	return archives, err
}

// GetLatestReadyArchive returns the newest usable archive of a document.
func GetLatestReadyArchive(url string, userID uint) (*DocumentArchive, error) {
	var a DocumentArchive
	err := DB.Where("url = ? AND user_id = ? AND status = ?", url, userID, ArchiveReady).
		Order("created_at DESC, id DESC").First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListReadyArchives returns a user's usable archives, newest first.
func ListReadyArchives(userID uint, limit, offset int) ([]*DocumentArchive, int64, error) {
	var total int64
	q := DB.Model(&DocumentArchive{}).Where("user_id = ? AND status = ?", userID, ArchiveReady)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var archives []*DocumentArchive
	err := q.Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&archives).Error
	return archives, total, err
}

// DeleteArchive removes one archive record and reports whether it existed.
func DeleteArchive(id, userID uint) (bool, error) {
	res := DB.Where("id = ? AND user_id = ?", id, userID).Delete(&DocumentArchive{})
	return res.RowsAffected > 0, res.Error
}

// DeleteArchives removes the archive records of a document and returns them
// so their files can be removed.
func DeleteArchives(url string, userID uint) ([]*DocumentArchive, error) {
	var archives []*DocumentArchive
	if err := DB.Where("url = ? AND user_id = ?", url, userID).Find(&archives).Error; err != nil {
		return nil, err
	}
	if len(archives) == 0 {
		return nil, nil
	}
	ids := make([]uint, len(archives))
	for i, a := range archives {
		ids[i] = a.ID
	}
	return archives, DB.Where("id IN ?", ids).Delete(&DocumentArchive{}).Error
}

// MoveArchives assigns the archives of a document to a new owner.
func MoveArchives(url string, fromUserID, toUserID uint) error {
	if fromUserID == toUserID {
		return nil
	}
	return DB.Model(&DocumentArchive{}).Where("url = ? AND user_id = ?", url, fromUserID).Update("user_id", toUserID).Error
}
