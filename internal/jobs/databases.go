package jobs

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"dumpersg/internal/core"
)

func (m *Manager) runOptionalCatalog(ctx context.Context, x *execution, cmd core.Command, catalog optionalCatalog) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.mu.Lock()
		optional, needed := catalog.optionalCommand(cmd)
		m.mu.Unlock()
		if !needed {
			return nil
		}
		err := m.executor.Run(ctx, optional, func(level, text string) {
			m.mu.Lock()
			defer m.mu.Unlock()
			if m.active != x {
				return
			}
			if level == "data" {
				catalog.consumeOptional(text)
			} else {
				m.eventLocked(x, level, text)
			}
		})
		// An optional lookup must still respect cancellation and container quarantine.
		if errors.Is(err, core.ErrTerminationUnconfirmed) || errors.Is(err, context.Canceled) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.mu.Lock()
		previousWarning := ""
		if databases, ok := catalog.(*databaseCatalog); ok {
			previousWarning = databases.result.Warning
		}
		catalog.optionalFinished(err)
		if databases, ok := catalog.(*databaseCatalog); ok && databases.result.Warning != previousWarning {
			m.eventLocked(x, "warning", databases.result.Warning)
		}
		m.mu.Unlock()
	}
}

// Collectors are accessed only while Manager.mu is held.
type resultCatalog interface {
	consume(string)
	failure() error
	finish() string
	release(bool)
}

type optionalCatalog interface {
	optionalCommand(core.Command) (core.Command, bool)
	consumeOptional(string)
	optionalFinished(error)
}

type databaseCatalog struct {
	result     core.DatabaseCatalog
	names      map[string]bool
	groups     map[int64]string
	err        error
	groupErr   error
	lookup     int
	companies  map[int64][]core.CompanyInfo
	companyIDs map[int64]bool
	companyErr error
}

func (c *databaseCatalog) consume(line string) {
	if c.err != nil {
		return
	}
	if len(c.result.Databases) >= core.MaxCatalogDatabases {
		c.err = fmt.Errorf("catálogo excede o limite de %d bancos; lista não será retornada parcialmente", core.MaxCatalogDatabases)
		return
	}
	info, err := core.ParseDatabaseInfo(line)
	if err != nil {
		c.err = err
		return
	}
	if c.names[info.Name] {
		c.err = fmt.Errorf("catálogo de bancos com nome duplicado")
		return
	}
	c.names[info.Name] = true
	c.result.Databases = append(c.result.Databases, info)
}

func (c *databaseCatalog) failure() error { return c.err }

func (c *databaseCatalog) optionalCommand(cmd core.Command) (core.Command, bool) {
	if c.lookup >= 2 {
		return core.Command{}, false
	}
	for _, info := range c.result.Databases {
		if strings.HasPrefix(info.Name, "sommusgestor_") {
			if c.lookup == 1 {
				return core.CompanyListCommand(cmd), true
			}
			return core.GroupListCommand(cmd), true
		}
	}
	return core.Command{}, false
}

func (c *databaseCatalog) consumeOptional(line string) {
	if c.lookup == 1 {
		c.consumeCompany(line)
		return
	}
	if c.groupErr != nil {
		return
	}
	if len(c.groups) >= core.MaxCatalogDatabases {
		c.groupErr = fmt.Errorf("catálogo de grupos excede o limite")
		return
	}
	info, err := core.ParseGroupInfo(line)
	if err != nil {
		c.groupErr = err
		return
	}
	if _, exists := c.groups[info.ID]; exists {
		c.groupErr = fmt.Errorf("catálogo de grupos com ID duplicado")
		return
	}
	c.groups[info.ID] = info.Name
}

func (c *databaseCatalog) optionalFinished(err error) {
	defer func() { c.lookup++ }()
	if c.lookup == 1 {
		if err != nil || c.companyErr != nil {
			c.companies = nil
			if c.result.Warning != "" {
				c.result.Warning += " "
			}
			c.result.Warning += "Não foi possível consultar as empresas em sommusgestor. A pesquisa por empresa está indisponível; os bancos e nomes dos grupos consultados continuam disponíveis."
		}
		return
	}
	if err != nil || c.groupErr != nil {
		c.groups = nil
		c.result.Warning = "Não foi possível consultar os nomes dos grupos em sommusgestor. Os bancos disponíveis continuam listados pelo nome do database."
	}
}

func (c *databaseCatalog) consumeCompany(line string) {
	if c.companyErr != nil {
		return
	}
	if len(c.companyIDs) >= core.MaxCatalogDatabases {
		c.companyErr = fmt.Errorf("catálogo de empresas excede o limite")
		return
	}
	info, err := core.ParseCompanyInfo(line)
	if err != nil {
		c.companyErr = err
		return
	}
	if c.companyIDs[info.ID] {
		c.companyErr = fmt.Errorf("catálogo de empresas com ID duplicado")
		return
	}
	c.companyIDs[info.ID] = true
	c.companies[info.GroupID] = append(c.companies[info.GroupID], info)
}

func (c *databaseCatalog) finish() string {
	for i := range c.result.Databases {
		info := &c.result.Databases[i]
		id, err := strconv.ParseInt(strings.TrimPrefix(info.Name, "sommusgestor_"), 10, 64)
		if err == nil && info.Name == "sommusgestor_"+strconv.FormatInt(id, 10) && (c.groups[id] != "" || len(c.companies[id]) > 0) {
			info.GroupID, info.GroupName = id, c.groups[id]
			info.Companies = c.companies[id]
		}
	}
	sort.Slice(c.result.Databases, func(i, j int) bool { return c.result.Databases[i].Name < c.result.Databases[j].Name })
	return fmt.Sprintf("Consulta concluída: %d bancos disponíveis.", len(c.result.Databases))
}

func (c *databaseCatalog) release(success bool) {
	c.names, c.groups = nil, nil
	c.companies, c.companyIDs = nil, nil
	if !success {
		c.result.Databases = nil
	}
}

func (m *Manager) StartDatabaseList(ctx context.Context, req core.DatabaseListRequest) (core.Job, error) {
	p, id, err := m.profile(ctx, req.ProfileID)
	if err != nil {
		return core.Job{}, err
	}
	cmd, err := core.BuildDatabaseList(p, m.cfg, id)
	if err != nil {
		return core.Job{}, err
	}
	catalog := &databaseCatalog{result: core.DatabaseCatalog{Databases: make([]core.DatabaseInfo, 0)}, names: make(map[string]bool), groups: make(map[int64]string), companies: make(map[int64][]core.CompanyInfo), companyIDs: make(map[int64]bool)}
	return m.startCollected(ctx, p, cmd, core.Job{ID: id, Kind: "database_list"}, catalog)
}

func (m *Manager) Databases(ctx context.Context, id string) (core.DatabaseCatalog, error) {
	if err := ctx.Err(); err != nil {
		return core.DatabaseCatalog{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	x := m.retained[id]
	if x == nil || x.job.Kind != "database_list" {
		return core.DatabaseCatalog{}, core.ErrNotFound
	}
	if x.job.Status != "succeeded" {
		return core.DatabaseCatalog{}, fmt.Errorf("catálogo disponível somente após consulta bem-sucedida: %w", core.ErrConflict)
	}
	catalog, ok := x.catalog.(*databaseCatalog)
	if !ok {
		return core.DatabaseCatalog{}, core.ErrNotFound
	}
	result := core.DatabaseCatalog{Warning: catalog.result.Warning, Databases: make([]core.DatabaseInfo, len(catalog.result.Databases))}
	copy(result.Databases, catalog.result.Databases)
	for i := range result.Databases {
		result.Databases[i].Companies = append([]core.CompanyInfo(nil), result.Databases[i].Companies...)
	}
	return result, nil
}
