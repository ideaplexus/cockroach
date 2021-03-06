// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Cockroach Community Licence (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://github.com/cockroachdb/cockroach/blob/master/pkg/ccl/LICENSE

package sqlccl

import (
	"github.com/cockroachdb/cockroach/pkg/sql/parser"
	"github.com/cockroachdb/cockroach/pkg/sql/sqlbase"
	"github.com/pkg/errors"
)

// descriptorsMatchingTargets returns the descriptors that match the targets. A
// database descriptor is included in this set if it matches the targets (or the
// session database) or if one of its tables matches the targets.
func descriptorsMatchingTargets(
	sessionDatabase string, descriptors []sqlbase.Descriptor, targets parser.TargetList,
) ([]sqlbase.Descriptor, error) {
	// TODO(dan): If the session search path starts including more than virtual
	// tables (as of 2017-01-12 it's only pg_catalog), then this method will
	// need to support it.

	starByDatabase := make(map[string]struct{}, len(targets.Databases))
	for _, d := range targets.Databases {
		starByDatabase[d.Normalize()] = struct{}{}
	}

	tablesByDatabase := make(map[string][]string, len(targets.Tables))
	for _, pattern := range targets.Tables {
		var err error
		pattern, err = pattern.NormalizeTablePattern()
		if err != nil {
			return nil, err
		}

		switch p := pattern.(type) {
		case *parser.TableName:
			if sessionDatabase != "" {
				if err := p.QualifyWithDatabase(sessionDatabase); err != nil {
					return nil, err
				}
			}
			db := p.DatabaseName.Normalize()
			tablesByDatabase[db] = append(tablesByDatabase[db], p.TableName.Normalize())
		case *parser.AllTablesSelector:
			if sessionDatabase != "" {
				if err := p.QualifyWithDatabase(sessionDatabase); err != nil {
					return nil, err
				}
			}
			starByDatabase[p.Database.Normalize()] = struct{}{}
		default:
			return nil, errors.Errorf("unknown pattern %T: %+v", pattern, pattern)
		}
	}

	databasesByID := make(map[sqlbase.ID]*sqlbase.DatabaseDescriptor, len(descriptors))
	var ret []sqlbase.Descriptor

	for _, desc := range descriptors {
		if dbDesc := desc.GetDatabase(); dbDesc != nil {
			databasesByID[dbDesc.ID] = dbDesc
			normalizedDBName := parser.ReNormalizeName(dbDesc.Name)
			if _, ok := starByDatabase[normalizedDBName]; ok {
				ret = append(ret, desc)
			} else if _, ok := tablesByDatabase[normalizedDBName]; ok {
				ret = append(ret, desc)
			}
		}
	}

	for _, desc := range descriptors {
		if tableDesc := desc.GetTable(); tableDesc != nil {
			dbDesc, ok := databasesByID[tableDesc.ParentID]
			if !ok {
				return nil, errors.Errorf("unknown ParentID: %d", tableDesc.ParentID)
			}
			normalizedDBName := parser.ReNormalizeName(dbDesc.Name)
			if _, ok := starByDatabase[normalizedDBName]; ok {
				ret = append(ret, desc)
			} else if tableNames, ok := tablesByDatabase[normalizedDBName]; ok {
				for _, tableName := range tableNames {
					if parser.ReNormalizeName(tableName) == parser.ReNormalizeName(tableDesc.Name) {
						ret = append(ret, desc)
						break
					}
				}
			}
		}
	}
	return ret, nil
}
