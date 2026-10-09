package jobs

import (
	"context"
	"fmt"

	"dumpersg/internal/core"
)

// Guarded by Manager.mu; initialized before execution can produce any output.
type tableCatalog struct {
	tables []core.TableInfo
	names  map[string]struct{}
	err    error
}

func (c *tableCatalog) consume(line string) {
	if c.err != nil {
		return
	}
	if len(c.tables) >= core.MaxCatalogTables {
		c.err = fmt.Errorf("database excede o limite de %d tabelas e views; catálogo não será retornado parcialmente", core.MaxCatalogTables)
		return
	}
	table, err := core.ParseTableInfo(line)
	if err != nil {
		c.err = err
		return
	}
	if _, exists := c.names[table.Name]; exists {
		c.err = fmt.Errorf("resposta de tabelas inválida: nome duplicado")
		return
	}
	c.names[table.Name] = struct{}{}
	c.tables = append(c.tables, table)
}

func (m *Manager) StartTableList(ctx context.Context, req core.TableListRequest) (core.Job, error) {
	p, id, err := m.profile(ctx, req.ProfileID)
	if err != nil {
		return core.Job{}, err
	}
	if req.Database == "" {
		req.Database = p.Database
	}
	cmd, err := core.BuildTableList(p, req, m.cfg, id)
	if err != nil {
		return core.Job{}, err
	}
	catalog := &tableCatalog{tables: make([]core.TableInfo, 0), names: make(map[string]struct{})}
	return m.startCollected(ctx, p, cmd, core.Job{ID: id, Kind: "table_list", Database: req.Database}, catalog)
}

func (m *Manager) Tables(ctx context.Context, id string) ([]core.TableInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	x := m.retained[id]
	if x == nil || x.job.Kind != "table_list" {
		return nil, core.ErrNotFound
	}
	if x.job.Status != "succeeded" {
		return nil, fmt.Errorf("catálogo disponível somente após consulta bem-sucedida: %w", core.ErrConflict)
	}
	if x.catalog == nil {
		return nil, core.ErrNotFound
	}
	tables := make([]core.TableInfo, len(x.catalog.tables))
	copy(tables, x.catalog.tables)
	core.SortTables(tables)
	return tables, nil
}
