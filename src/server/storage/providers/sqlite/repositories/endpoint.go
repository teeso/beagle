package repositories

import (
	"database/sql"
	"fmt"
	"github.com/blent/beagle/src/core/notification"
	"github.com/blent/beagle/src/server/storage"
	"github.com/blent/beagle/src/server/storage/providers/sqlite/repositories/mapping"
	"github.com/blent/beagle/src/server/utils"
	"github.com/pkg/errors"
	"strings"
)

const (
	endpointSelectQuery       = "SELECT id, name, url, method, headers FROM %s"
	endpointInsertQuery       = "INSERT INTO %s (name, url, method, headers) VALUES %s"
	endpointInsertValuesQuery = "(?, ?, ?, ?)"
	endpointUpdateQuery       = "UPDATE %s SET name=?, url=?, method=?, headers=? WHERE id=?"
	endpointDeleteQuery       = "DELETE FROM %s"
	endpointCountQuery        = "SELECT COUNT(id) from %s"
)

type (
	SQLiteEndpointRepository struct {
		tableName string
		db        *sql.DB
	}
)

func NewSQLiteEndpointRepository(tableName string, db *sql.DB) *SQLiteEndpointRepository {
	return &SQLiteEndpointRepository{tableName, db}
}

func (r *SQLiteEndpointRepository) Get(id uint64) (*notification.Endpoint, error) {
	if id == 0 {
		return nil, errors.New("id must be greater than 0")
	}

	stmt, err := r.db.Prepare(
		fmt.Sprintf(
			"%s WHERE id=? LIMIT 1",
			fmt.Sprintf(
				endpointSelectQuery,
				r.tableName,
			),
		),
	)

	if err != nil {
		return nil, err
	}

	defer stmt.Close()

	return mapping.ToEndpoint(stmt.QueryRow(id))
}

func (r *SQLiteEndpointRepository) Find(query *storage.EndpointQuery) ([]*notification.Endpoint, error) {
	args := make([]interface{}, 0, 5)
	findQuery := fmt.Sprintf(endpointSelectQuery, r.tableName)

	if query != nil {
		if query.Name != "" {
			findQuery += " WHERE"

			startsWith := strings.HasPrefix(query.Name, "*")
			endsWith := strings.HasSuffix(query.Name, "*")
			arg := query.Name

			if startsWith || endsWith {
				arg = strings.Replace(arg, "*", "", -1)

				if startsWith && endsWith {
					arg = "%" + arg + "%"
				} else if endsWith {
					arg = arg + "%"
				} else {
					arg = "%" + arg
				}

				findQuery += " name LIKE ?"
			} else {
				findQuery += " name = ?"
			}

			args = append(args, arg)
		}

		findQuery += " ORDER BY id"


		if query.Take > 0 {
			findQuery += " LIMIT ? OFFSET ?"

			args = append(args, query.Take, query.Skip)
		}
	} else {
		findQuery += " ORDER BY id"
	}

	fmt.Println(findQuery)

	stmt, err := r.db.Prepare(findQuery)

	if err != nil {
		return nil, err
	}

	defer stmt.Close()

	rows, err := stmt.Query(args...)

	if err != nil {
		return nil, err
	}

	return mapping.ToEndpoints(rows, query.Take)
}

func (r *SQLiteEndpointRepository) Count() (uint64, error) {
	queryStmt := fmt.Sprintf(endpointCountQuery, r.tableName)

	stmt, err := r.db.Prepare(queryStmt)

	if err != nil {
		return 0, err
	}

	defer stmt.Close()

	row := stmt.QueryRow()

	var count uint64

	err = row.Scan(&count)

	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *SQLiteEndpointRepository) Create(endpoint *notification.Endpoint, tx *sql.Tx) (uint64, error) {
	if endpoint == nil {
		return 0, errors.New("endpoint missed")
	}

	var id int64
	var err error

	if endpoint.Id > 0 {
		return 0, errors.New("endpoint already created")
	}

	tx, closeTx, err := storage.TryToBegin(r.db, tx)

	if err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare(
		fmt.Sprintf(endpointInsertQuery, r.tableName, endpointInsertValuesQuery),
	)

	if err != nil {
		return 0, storage.TryToRollback(tx, err, closeTx)
	}

	res, err := stmt.Exec(endpoint.Name, endpoint.Url, endpoint.Method, endpoint.Headers)

	if err != nil {
		return 0, storage.TryToRollback(tx, err, closeTx)
	}

	id, err = res.LastInsertId()

	if err != nil {
		return 0, storage.TryToRollback(tx, err, closeTx)
	}

	err = storage.TryToCommit(tx, closeTx)

	if err != nil {
		return 0, err
	}

	return uint64(id), nil
}

func (r *SQLiteEndpointRepository) Update(endpoint *notification.Endpoint, tx *sql.Tx) error {
	if endpoint == nil {
		return errors.New("endpoint missed")
	}

	var err error

	if endpoint.Id == 0 || endpoint.Id < 0 {
		return errors.New("endpoint not created yet")
	}

	tx, closeTx, err := storage.TryToBegin(r.db, tx)

	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		fmt.Sprintf(endpointUpdateQuery, r.tableName),
	)

	if err != nil {
		return storage.TryToRollback(tx, err, closeTx)
	}

	_, err = stmt.Exec(endpoint.Name, endpoint.Url, endpoint.Method, endpoint.Headers, endpoint.Id)

	if err != nil {
		return storage.TryToRollback(tx, err, closeTx)
	}

	return storage.TryToCommit(tx, closeTx)
}

func (r *SQLiteEndpointRepository) Delete(id uint64, tx *sql.Tx) error {
	if id == 0 {
		return errors.New("id must be greater than 0")
	}

	var err error

	tx, closeTx, err := storage.TryToBegin(r.db, tx)

	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		fmt.Sprintf(
			"%s WHERE id=?",
			fmt.Sprintf(endpointDeleteQuery, r.tableName),
		),
	)

	if err != nil {
		return storage.TryToRollback(tx, err, closeTx)
	}

	_, err = stmt.Exec(id)

	if err != nil {
		return storage.TryToRollback(tx, err, closeTx)
	}

	return storage.TryToCommit(tx, closeTx)
}

func (r *SQLiteEndpointRepository) DeleteMany(ids []uint64, tx *sql.Tx) error {
	if len(ids) == 0 {
		return errors.New("passed empty list of ids")
	}

	var err error

	tx, closeTx, err := storage.TryToBegin(r.db, tx)

	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		fmt.Sprintf(
			"%s WHERE id IN (%s)",
			fmt.Sprintf(endpointDeleteQuery, r.tableName),
			utils.JoinUintSlice(ids, ", "),
		),
	)

	if err != nil {
		return storage.TryToRollback(tx, err, closeTx)
	}

	_, err = stmt.Exec()

	if err != nil {
		return storage.TryToRollback(tx, err, closeTx)
	}

	return storage.TryToCommit(tx, closeTx)
}
