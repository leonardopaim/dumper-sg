package core

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"
)

const maxBackupMetadataBytes = 8 << 20
const maxBackupFiles = 100000
const maxSchemaBytes = 256 << 10

type backupEntry struct {
	info   BackupTableInfo
	stem   string
	schema bool
}

// ListBackupTables reads only the local backup. Sizes are bytes stored on disk,
// including compression, not estimates from the source database.
func ListBackupTables(path string) ([]BackupTableInfo, error) {
	dir, err := ExistingDirectory(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("abrir backup: %w", err)
	}
	defer root.Close()
	files, err := backupFiles(root)
	if err != nil {
		return nil, err
	}
	metadata, err := readBackupFile(root, files, "metadata", maxBackupMetadataBytes)
	if err != nil {
		return nil, fmt.Errorf("metadata inválido: %w", err)
	}
	if !utf8.Valid(metadata) {
		return nil, fmt.Errorf("metadata deve conter UTF-8 válido")
	}
	complete := false
	for _, line := range strings.Split(string(metadata), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# Finished dump at:") || strings.HasPrefix(line, "Finished dump at:") {
			complete = true
		}
	}
	if !complete {
		return nil, fmt.Errorf("backup incompleto: metadata não confirma o fim do dump")
	}
	entries, databases, err := parseBackupMetadata(string(metadata))
	if err != nil {
		return nil, err
	}
	budget := maxBackupMetadataBytes
	if entries == nil {
		entries, databases, err = legacyBackupEntries(root, files, &budget)
		if err != nil {
			return nil, err
		}
	}
	// Database aliases also require the CREATE DATABASE mapping. Unlike table
	// aliases, MyDumper's INI metadata does not carry a real_database_name key.
	for alias, database := range databases {
		if strings.HasPrefix(alias, "mydumper_") && alias == database {
			name, err := databaseFromSchema(root, files, alias, &budget)
			if err != nil {
				return nil, err
			}
			databases[alias] = name
			for _, entry := range entries {
				if entry.info.Database == database {
					entry.info.Database = name
				}
			}
		}
	}
	for name, stat := range files {
		plain := stripBackupCompression(name)
		if plain == "metadata" || !strings.HasSuffix(plain, ".sql") && !strings.HasSuffix(plain, ".dat") {
			continue
		}
		stem, schema, tableFile := tableFileStem(plain)
		if !tableFile {
			global := false
			for _, suffix := range []string{"-schema-create.sql", "-schema-post.sql"} {
				if strings.HasSuffix(plain, suffix) && databases[strings.TrimSuffix(plain, suffix)] != "" {
					global = true
				}
			}
			if global {
				continue
			}
			return nil, fmt.Errorf("arquivo SQL ou de dados não reconhecido no backup: %s", name)
		}
		entry := entries[stem]
		if entry == nil && !schema {
			// A chunk may have both part and subpart suffixes. Prefer the exact
			// known stem before stripping another numeric component.
			if idx := strings.LastIndexByte(stem, '.'); idx >= 0 {
				part := stem[idx+1:]
				if len(part) == 5 && strings.Trim(part, "0123456789") == "" {
					entry = entries[stem[:idx]]
				}
			}
		}
		if entry == nil && strings.HasSuffix(plain, "-schema-triggers.sql") && databases[stem] != "" {
			continue
		}
		if entry == nil {
			return nil, fmt.Errorf("arquivo de tabela sem correspondência no metadata: %s", name)
		}
		if stat.Size() < 0 || uint64(stat.Size()) > math.MaxUint64-entry.info.SizeBytes {
			return nil, fmt.Errorf("tamanho do backup excede o limite")
		}
		entry.info.SizeBytes += uint64(stat.Size())
		entry.schema = entry.schema || schema
		if strings.HasSuffix(plain, "-schema-view.sql") {
			entry.info.TableType = "VIEW"
		}
	}
	result := make([]BackupTableInfo, 0, len(entries))
	seen := make(map[TableReference]bool, len(entries))
	for _, entry := range entries {
		if !entry.schema {
			return nil, fmt.Errorf("backup incompleto: tabela %s.%s não possui schema", entry.info.Database, entry.info.Name)
		}
		ref := TableReference{Database: entry.info.Database, Name: entry.info.Name}
		if seen[ref] {
			return nil, fmt.Errorf("metadata contém tabela repetida: %s.%s", ref.Database, ref.Name)
		}
		seen[ref] = true
		result = append(result, entry.info)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SizeBytes != result[j].SizeBytes {
			return result[i].SizeBytes > result[j].SizeBytes
		}
		if result[i].Database != result[j].Database {
			return result[i].Database < result[j].Database
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func backupFiles(root *os.Root) (map[string]os.FileInfo, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	files := make(map[string]os.FileInfo)
	count := 0
	for {
		batch, err := dir.ReadDir(256)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("ler backup: %w", err)
		}
		for _, entry := range batch {
			count++
			if count > maxBackupFiles {
				return nil, fmt.Errorf("backup excede o limite de 100000 arquivos")
			}
			stat, statErr := root.Lstat(entry.Name())
			if statErr != nil {
				return nil, statErr
			}
			if !stat.Mode().IsRegular() {
				return nil, fmt.Errorf("backup deve conter somente arquivos regulares, sem links: %s", entry.Name())
			}
			files[entry.Name()] = stat
		}
		if err == io.EOF {
			break
		}
	}
	return files, nil
}

func readBackupFile(root *os.Root, files map[string]os.FileInfo, name string, limit int) ([]byte, error) {
	expected := files[name]
	if expected == nil || expected.Size() > int64(limit) {
		return nil, fmt.Errorf("arquivo %s ausente ou maior que %d bytes", name, limit)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.Mode().IsRegular() || !os.SameFile(expected, stat) {
		return nil, fmt.Errorf("arquivo do backup mudou durante a leitura")
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, fmt.Errorf("não foi possível ler %s dentro do limite", name)
	}
	return data, nil
}

func parseBackupMetadata(data string) (map[string]*backupEntry, map[string]string, error) {
	entries := make(map[string]*backupEntry)
	databases := make(map[string]string)
	var current *backupEntry
	var fields map[string]bool
	ini := false
	finish := func() error {
		if current != nil && (!fields["rows"] || !fields["real_table_name"]) {
			return fmt.Errorf("metadata incompleto: tabela sem nome real ou contagem de linhas")
		}
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, 4096), maxBackupMetadataBytes)
	for scanner.Scan() {
		rawLine := strings.TrimSuffix(scanner.Text(), "\r")
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if err := finish(); err != nil {
				return nil, nil, err
			}
			ini = true
			current = nil
			if !strings.HasSuffix(line, "]") {
				return nil, nil, fmt.Errorf("seção de metadata inválida")
			}
			group := strings.TrimSpace(line[1 : len(line)-1])
			if group == "" {
				return nil, nil, fmt.Errorf("seção de metadata vazia")
			}
			if group[0] != '`' && group[0] != '"' {
				continue
			}
			ids, err := quotedBackupIdentifiers(group)
			if err != nil || len(ids) > 2 {
				return nil, nil, fmt.Errorf("identificadores inválidos no metadata")
			}
			if err := ValidateDatabase(ids[0]); err != nil {
				return nil, nil, err
			}
			databases[ids[0]] = ids[0]
			if len(ids) == 1 {
				continue
			}
			if err := ValidateTableName(ids[1]); err != nil {
				return nil, nil, err
			}
			stem := ids[0] + "." + ids[1]
			if entries[stem] != nil {
				return nil, nil, fmt.Errorf("seção de tabela repetida ou ambígua no metadata")
			}
			current = &backupEntry{stem: stem, info: BackupTableInfo{Database: ids[0], TableType: "BASE TABLE", fileDatabase: ids[0], fileTable: ids[1]}}
			entries[stem] = current
			if len(entries) > MaxCatalogTables {
				return nil, nil, fmt.Errorf("backup excede o limite de 10000 tabelas")
			}
			fields = make(map[string]bool)
			continue
		}
		key, value, ok := strings.Cut(rawLine, "=")
		if !ok {
			if !ini {
				continue
			}
			return nil, nil, fmt.Errorf("linha de metadata inválida")
		}
		if current == nil {
			continue
		}
		key = strings.TrimSpace(key)
		rawValue := value
		value = strings.TrimSpace(value)
		if fields[key] {
			return nil, nil, fmt.Errorf("campo repetido no metadata de tabela")
		}
		fields[key] = true
		switch key {
		case "real_table_name":
			// MyDumper writes the literal value (newline-protected controls are
			// deliberately unsupported identifiers), not a quoted SQL string.
			if err := ValidateTableName(rawValue); err != nil {
				return nil, nil, err
			}
			current.info.Name = rawValue
		case "rows":
			n, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return nil, nil, fmt.Errorf("contagem de linhas inválida no metadata")
			}
			current.info.Rows = n
		case "is_view":
			if value != "0" && value != "1" {
				return nil, nil, fmt.Errorf("is_view inválido no metadata")
			}
			if value == "1" {
				current.info.TableType = "VIEW"
			}
		case "is_sequence":
			if value == "1" {
				return nil, nil, fmt.Errorf("sequências não são suportadas no catálogo de tabelas")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("metadata excede limite de leitura")
	}
	if err := finish(); err != nil {
		return nil, nil, err
	}
	if !ini {
		return nil, nil, nil
	}
	if len(databases) == 0 {
		return nil, nil, fmt.Errorf("metadata não contém catálogo de databases reconhecido")
	}
	return entries, databases, nil
}

// Doubled identifier quotes are escapes. Dots inside quotes stay in the name.
func quotedBackupIdentifiers(value string) ([]string, error) {
	var ids []string
	for {
		value = strings.TrimSpace(value)
		if len(value) == 0 || value[0] != '`' && value[0] != '"' {
			return nil, fmt.Errorf("identificador deve estar entre aspas")
		}
		quote := value[0]
		var name strings.Builder
		i := 1
		closed := false
		for i < len(value) {
			if value[i] == quote {
				if i+1 < len(value) && value[i+1] == quote {
					name.WriteByte(quote)
					i += 2
					continue
				}
				i++
				closed = true
				break
			}
			name.WriteByte(value[i])
			i++
		}
		if !closed {
			return nil, fmt.Errorf("identificador incompleto")
		}
		ids = append(ids, name.String())
		value = strings.TrimSpace(value[i:])
		if value == "" {
			return ids, nil
		}
		if value[0] != '.' || len(ids) > 1 {
			return nil, fmt.Errorf("identificador inválido")
		}
		value = value[1:]
	}
}

func stripBackupCompression(name string) string {
	for _, suffix := range []string{".gz", ".zst"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix)
		}
	}
	return name
}

func tableFileStem(name string) (string, bool, bool) {
	for _, suffix := range []string{"-schema-create.sql", "-schema-post.sql", "-schema-triggers.sql"} {
		// Database triggers have no table component. Table triggers are handled
		// below by exact metadata stem lookup.
		if strings.HasSuffix(name, suffix) && suffix != "-schema-triggers.sql" {
			return "", false, false
		}
	}
	for _, suffix := range []string{"-schema-view.sql", "-schema.sql", "-schema-triggers.sql"} {
		if strings.HasSuffix(name, suffix) {
			return strings.TrimSuffix(name, suffix), suffix != "-schema-triggers.sql", true
		}
	}
	base := strings.TrimSuffix(strings.TrimSuffix(name, ".sql"), ".dat")
	idx := strings.LastIndexByte(base, '.')
	if idx < 0 {
		return "", false, false
	}
	part := base[idx+1:]
	if len(part) != 5 || strings.Trim(part, "0123456789") != "" {
		return "", false, false
	}
	return base[:idx], false, true
}

var legacyCreateDatabase = regexp.MustCompile(`(?i)CREATE\s+DATABASE\s+(?:IF\s+NOT\s+EXISTS\s+|/\*!\d+\s+IF\s+NOT\s+EXISTS\s*\*/\s*)?`)
var legacyCreateTable = regexp.MustCompile(`(?i)CREATE\s+(?:TABLE|(?:OR\s+REPLACE\s+)?VIEW)\s+(?:IF\s+NOT\s+EXISTS\s+)?`)

func schemaIdentifier(data []byte, pattern *regexp.Regexp) (string, error) {
	where := pattern.FindIndex(data)
	if where == nil {
		return "", fmt.Errorf("schema não contém identificador CREATE reconhecido")
	}
	value := strings.TrimSpace(string(data[where[1]:]))
	// Find one quoted SQL identifier without interpreting column definitions.
	if len(value) == 0 || value[0] != '`' && value[0] != '"' {
		return "", fmt.Errorf("schema deve conter identificador entre aspas")
	}
	quote := value[0]
	for i := 1; i < len(value); i++ {
		if value[i] == quote {
			if i+1 < len(value) && value[i+1] == quote {
				i++
				continue
			}
			ids, err := quotedBackupIdentifiers(value[:i+1])
			if err != nil {
				return "", err
			}
			return ids[0], nil
		}
	}
	return "", fmt.Errorf("identificador incompleto no schema")
}

func readBackupSchema(root *os.Root, files map[string]os.FileInfo, name string, budget *int) ([]byte, error) {
	data, err := readBackupFile(root, files, name, maxSchemaBytes)
	if err != nil {
		return nil, err
	}
	*budget -= len(data)
	if *budget < 0 {
		return nil, fmt.Errorf("schemas excedem o limite total de leitura de 8 MiB")
	}
	if strings.HasSuffix(name, ".gz") {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("schema gzip inválido")
		}
		data, err = io.ReadAll(io.LimitReader(gz, maxSchemaBytes+1))
		closeErr := gz.Close()
		if err != nil || closeErr != nil || len(data) > maxSchemaBytes {
			return nil, fmt.Errorf("schema gzip inválido ou maior que 256 KiB")
		}
		*budget -= len(data)
	} else if strings.HasSuffix(name, ".zst") {
		decoder, err := zstd.NewReader(bytes.NewReader(data), zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(8<<20), zstd.WithDecoderMaxWindow(8<<20), zstd.WithDecodeBuffersBelow(0))
		if err != nil {
			return nil, fmt.Errorf("schema zstd inválido")
		}
		data, err = io.ReadAll(io.LimitReader(decoder, maxSchemaBytes+1))
		decoder.Close()
		if err != nil || len(data) > maxSchemaBytes {
			return nil, fmt.Errorf("schema zstd inválido ou maior que 256 KiB (janela máxima 8 MiB)")
		}
		*budget -= len(data)
	}
	if *budget < 0 {
		return nil, fmt.Errorf("schemas excedem o limite total de leitura de 8 MiB")
	}
	return data, nil
}

func databaseFromSchema(root *os.Root, files map[string]os.FileInfo, alias string, budget *int) (string, error) {
	for _, suffix := range []string{".sql", ".sql.gz", ".sql.zst"} {
		name := alias + "-schema-create" + suffix
		if files[name] == nil {
			continue
		}
		data, err := readBackupSchema(root, files, name, budget)
		if err != nil {
			return "", err
		}
		name, err = schemaIdentifier(data, legacyCreateDatabase)
		if err != nil {
			return "", err
		}
		return name, ValidateDatabase(name)
	}
	return "", fmt.Errorf("backup não contém schema-create para database %s", alias)
}

func legacyBackupEntries(root *os.Root, files map[string]os.FileInfo, budget *int) (map[string]*backupEntry, map[string]string, error) {
	entries := make(map[string]*backupEntry)
	databases := make(map[string]string)
	for file := range files {
		plain := stripBackupCompression(file)
		if !strings.HasSuffix(plain, "-schema-create.sql") {
			continue
		}
		alias := strings.TrimSuffix(plain, "-schema-create.sql")
		name, err := databaseFromSchema(root, files, alias, budget)
		if err != nil {
			return nil, nil, err
		}
		databases[alias] = name
	}
	for file := range files {
		plain := stripBackupCompression(file)
		stem, schema, table := tableFileStem(plain)
		if !schema || !table {
			continue
		}
		var db string
		matches := 0
		for alias := range databases {
			if strings.HasPrefix(stem, alias+".") {
				db = alias
				matches++
			}
		}
		if matches != 1 {
			return nil, nil, fmt.Errorf("metadata antigo: database do schema é ausente ou ambíguo")
		}
		data, err := readBackupSchema(root, files, file, budget)
		if err != nil {
			return nil, nil, err
		}
		name, err := schemaIdentifier(data, legacyCreateTable)
		if err != nil {
			return nil, nil, err
		}
		if err := ValidateTableName(name); err != nil {
			return nil, nil, err
		}
		if old := entries[stem]; old != nil {
			if old.info.Name != name {
				return nil, nil, fmt.Errorf("schemas inconsistentes no backup")
			}
			continue
		}
		typeName := "BASE TABLE"
		if strings.HasSuffix(plain, "-schema-view.sql") {
			typeName = "VIEW"
		}
		entries[stem] = &backupEntry{stem: stem, info: BackupTableInfo{Database: databases[db], Name: name, TableType: typeName, fileDatabase: db, fileTable: strings.TrimPrefix(stem, db+".")}}
		if len(entries) > MaxCatalogTables {
			return nil, nil, fmt.Errorf("backup excede o limite de 10000 tabelas")
		}
	}
	if len(databases) == 0 {
		return nil, nil, fmt.Errorf("metadata antigo não contém catálogo; requer schemas SQL, gzip ou zstd com CREATE DATABASE e CREATE TABLE")
	}
	return entries, databases, nil
}
