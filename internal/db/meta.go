package db

import (
	"database/sql"
	"log"
	"os"

	"roodox_server/internal/fs"
)

type MetaStore struct {
	db *sql.DB
}

func NewMetaStore(database *DB) (*MetaStore, error) {
	return &MetaStore{db: database.Sql}, nil
}

func (m *MetaStore) GetMeta(path string) (*fs.Meta, error) {
	row := m.db.QueryRow(
		`SELECT path, is_dir, size, mtime_unix, hash, version FROM meta WHERE path = ?`,
		path,
	)

	var (
		meta   fs.Meta
		isDirI int
	)
	err := row.Scan(&meta.Path, &isDirI, &meta.Size, &meta.MtimeUnix, &meta.Hash, &meta.Version)
	if err == sql.ErrNoRows {
		return nil, os.ErrNotExist
	}
	if err != nil {
		log.Printf("component=db store=meta op=GetMeta path=%q error=%q", path, err.Error())
		return nil, err
	}
	meta.IsDir = isDirI != 0
	return &meta, nil
}

func (m *MetaStore) SetMeta(meta *fs.Meta) error {
	isDirI := 0
	if meta.IsDir {
		isDirI = 1
	}
	_, err := m.db.Exec(`
INSERT INTO meta(path, is_dir, size, mtime_unix, hash, version)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
    is_dir     = excluded.is_dir,
    size       = excluded.size,
    mtime_unix = excluded.mtime_unix,
    hash       = excluded.hash,
    version    = excluded.version;
`,
		meta.Path, isDirI, meta.Size, meta.MtimeUnix, meta.Hash, meta.Version,
	)
	if err != nil {
		log.Printf("component=db store=meta op=SetMeta path=%q error=%q", meta.Path, err.Error())
	}
	return err
}

func (m *MetaStore) DeleteMeta(path string) error {
	_, err := m.db.Exec(`DELETE FROM meta WHERE path = ?`, path)
	if err != nil {
		log.Printf("component=db store=meta op=DeleteMeta path=%q error=%q", path, err.Error())
	}
	return err
}

func (m *MetaStore) AllMetas() ([]*fs.Meta, error) {
	rows, err := m.db.Query(`SELECT path, is_dir, size, mtime_unix, hash, version FROM meta`)
	if err != nil {
		log.Printf("component=db store=meta op=AllMetas error=%q", err.Error())
		return nil, err
	}
	defer rows.Close()

	var list []*fs.Meta
	for rows.Next() {
		var meta fs.Meta
		var isDirI int
		if err := rows.Scan(&meta.Path, &isDirI, &meta.Size, &meta.MtimeUnix, &meta.Hash, &meta.Version); err != nil {
			log.Printf("component=db store=meta op=AllMetas.scan error=%q", err.Error())
			return nil, err
		}
		meta.IsDir = isDirI != 0
		list = append(list, &meta)
	}
	if err := rows.Err(); err != nil {
		log.Printf("component=db store=meta op=AllMetas.rows error=%q", err.Error())
		return nil, err
	}
	return list, nil
}
