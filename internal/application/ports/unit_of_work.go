package ports

import "database/sql"

type UnitOfWork interface {
	WithTx(fn func(tx *sql.Tx) error) error
}