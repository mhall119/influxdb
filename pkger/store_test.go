package pkger_test

import (
	"context"
	"testing"
	"time"

	"github.com/influxdata/influxdb/v2"
	"github.com/influxdata/influxdb/v2/inmem"
	"github.com/influxdata/influxdb/v2/pkger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreKV(t *testing.T) {
	inMemStore := inmem.NewKVStore()

	stackStub := func(id, orgID influxdb.ID) pkger.Stack {
		now := time.Time{}.Add(10 * 365 * 24 * time.Hour)
		urls := []string{
			"http://example.com",
			"http://abc.gov",
		}
		return pkger.Stack{
			ID:          id,
			OrgID:       orgID,
			Name:        "threeve",
			Description: "desc",
			CRUDLog: influxdb.CRUDLog{
				CreatedAt: now,
				UpdatedAt: now.Add(time.Hour),
			},
			Sources: urls,
			URLs:    urls,
			Resources: []pkger.StackResource{
				{
					APIVersion: pkger.APIVersion,
					ID:         9000,
					Kind:       pkger.KindBucket,
					PkgName:    "buzz lightyear",
					Associations: []pkger.StackResourceAssociation{{
						Kind:    pkger.KindLabel,
						PkgName: "foo_label",
					}},
				},
				{
					APIVersion: pkger.APIVersion,
					ID:         333,
					Kind:       pkger.KindBucket,
					PkgName:    "beyond",
				},
			},
		}
	}

	t.Run("create a stack", func(t *testing.T) {
		defer inMemStore.Flush(context.Background())

		storeKV := pkger.NewStoreKV(inMemStore)

		const orgID = 333
		seedEntities(t, storeKV, pkger.Stack{
			ID:    1,
			OrgID: orgID,
		})

		t.Run("with no ID collisions creates successfully", func(t *testing.T) {
			expected := stackStub(3, orgID)

			err := storeKV.CreateStack(context.Background(), expected)
			require.NoError(t, err)

			readStackEqual(t, storeKV, expected)
		})

		t.Run("with ID collisions fails with conflict error", func(t *testing.T) {
			for _, id := range []influxdb.ID{2, 3} {
				err := storeKV.CreateStack(context.Background(), pkger.Stack{
					ID:    1,
					OrgID: orgID,
				})
				require.Errorf(t, err, "id=%d", id)
				assert.Equalf(t, influxdb.EConflict, influxdb.ErrorCode(err), "id=%d", id)
			}
		})
	})

	t.Run("list stacks", func(t *testing.T) {
		defer inMemStore.Flush(context.Background())

		storeKV := pkger.NewStoreKV(inMemStore)

		const orgID1 = 1
		const orgID2 = 2
		seedEntities(t, storeKV,
			pkger.Stack{
				ID:    1,
				OrgID: orgID1,
				Name:  "first_name",
			},
			pkger.Stack{
				ID:    2,
				OrgID: orgID2,
				Name:  "first_name",
			},
			pkger.Stack{
				ID:    3,
				OrgID: orgID1,
				Name:  "second_name",
			},
			pkger.Stack{
				ID:    4,
				OrgID: orgID2,
				Name:  "second_name",
			},
		)

		tests := []struct {
			name     string
			orgID    influxdb.ID
			filter   pkger.ListFilter
			expected []pkger.Stack
		}{
			{
				name:  "by org id",
				orgID: orgID1,
				expected: []pkger.Stack{
					{
						ID:    1,
						OrgID: orgID1,
						Name:  "first_name",
					},
					{
						ID:    3,
						OrgID: orgID1,
						Name:  "second_name",
					},
				},
			},
			{
				name:  "by stack ids",
				orgID: orgID1,
				filter: pkger.ListFilter{
					StackIDs: []influxdb.ID{1, 3},
				},
				expected: []pkger.Stack{
					{
						ID:    1,
						OrgID: orgID1,
						Name:  "first_name",
					},
					{
						ID:    3,
						OrgID: orgID1,
						Name:  "second_name",
					},
				},
			},
			{
				name:  "by stack ids skips ids that belong to different organization",
				orgID: orgID1,
				filter: pkger.ListFilter{
					StackIDs: []influxdb.ID{1, 2, 4},
				},
				expected: []pkger.Stack{{
					ID:    1,
					OrgID: orgID1,
					Name:  "first_name",
				}},
			},
			{
				name:  "stack ids that do not exist are skipped",
				orgID: orgID1,
				filter: pkger.ListFilter{
					StackIDs: []influxdb.ID{1, 9000},
				},
				expected: []pkger.Stack{{
					ID:    1,
					OrgID: orgID1,
					Name:  "first_name",
				}},
			},
			{
				name:  "by name",
				orgID: orgID1,
				filter: pkger.ListFilter{
					Names: []string{"first_name"},
				},
				expected: []pkger.Stack{{
					ID:    1,
					OrgID: orgID1,
					Name:  "first_name",
				}},
			},
			{
				name:  "by name and id",
				orgID: orgID1,
				filter: pkger.ListFilter{
					StackIDs: []influxdb.ID{3},
					Names:    []string{"first_name"},
				},
				expected: []pkger.Stack{
					{
						ID:    1,
						OrgID: orgID1,
						Name:  "first_name",
					},
					{
						ID:    3,
						OrgID: orgID1,
						Name:  "second_name",
					},
				},
			},
		}

		for _, tt := range tests {
			fn := func(t *testing.T) {
				stacks, err := storeKV.ListStacks(context.Background(), tt.orgID, tt.filter)
				require.NoError(t, err)
				assert.Equal(t, tt.expected, stacks)
			}

			t.Run(tt.name, fn)
		}
	})

	t.Run("read a stack", func(t *testing.T) {
		defer inMemStore.Flush(context.Background())

		storeKV := pkger.NewStoreKV(inMemStore)

		expected := stackStub(1, 3)

		seedEntities(t, storeKV, expected)

		t.Run("with valid ID returns stack successfully", func(t *testing.T) {
			readStackEqual(t, storeKV, expected)
		})

		t.Run("when no match found fails with not found error", func(t *testing.T) {
			unmatchedID := influxdb.ID(3000)
			_, err := storeKV.ReadStackByID(context.Background(), unmatchedID)
			require.Error(t, err)
			assert.Equal(t, influxdb.ENotFound, influxdb.ErrorCode(err))
		})
	})

	t.Run("update a stack", func(t *testing.T) {
		defer inMemStore.Flush(context.Background())

		storeKV := pkger.NewStoreKV(inMemStore)

		const orgID = 3
		const id = 3
		expected := stackStub(id, orgID)

		seedEntities(t, storeKV, expected)

		t.Run("with valid ID updates stack successfully", func(t *testing.T) {
			updateStack := expected
			updateStack.Resources = append(updateStack.Resources, pkger.StackResource{
				APIVersion: pkger.APIVersion,
				ID:         333,
				Kind:       pkger.KindBucket,
				PkgName:    "beyond",
			})

			err := storeKV.UpdateStack(context.Background(), updateStack)
			require.NoError(t, err)

			readStackEqual(t, storeKV, updateStack)
		})

		t.Run("when no match found fails with not found error", func(t *testing.T) {
			unmatchedID := influxdb.ID(3000)
			err := storeKV.UpdateStack(context.Background(), pkger.Stack{
				ID:    unmatchedID,
				OrgID: orgID,
			})
			require.Error(t, err)
			assert.Equalf(t, influxdb.ENotFound, influxdb.ErrorCode(err), "err: %s", err)
		})

		t.Run("when org id does not match fails with unprocessable entity error", func(t *testing.T) {
			err := storeKV.UpdateStack(context.Background(), pkger.Stack{
				ID:    id,
				OrgID: orgID + 9000,
			})
			require.Error(t, err)
			assert.Equalf(t, influxdb.EUnprocessableEntity, influxdb.ErrorCode(err), "err: %s", err)
		})
	})

	t.Run("delete a stack", func(t *testing.T) {
		defer inMemStore.Flush(context.Background())

		storeKV := pkger.NewStoreKV(inMemStore)

		const orgID = 3
		expected := stackStub(1, orgID)

		seedEntities(t, storeKV, expected)

		t.Run("with valid ID deletes stack successfully", func(t *testing.T) {
			err := storeKV.DeleteStack(context.Background(), expected.ID)
			require.NoError(t, err)

			_, err = storeKV.ReadStackByID(context.Background(), expected.ID)
			require.Error(t, err)
			errCodeEqual(t, influxdb.ENotFound, err)
		})

		t.Run("when no match found fails with not found error", func(t *testing.T) {
			unmatchedID := influxdb.ID(3000)
			err := storeKV.DeleteStack(context.Background(), unmatchedID)
			require.Error(t, err)
			errCodeEqual(t, influxdb.ENotFound, err)
		})
	})
}

func readStackEqual(t *testing.T, store pkger.Store, expected pkger.Stack) {
	t.Helper()

	stack, err := store.ReadStackByID(context.Background(), expected.ID)
	require.NoError(t, err)
	assert.Equal(t, expected, stack)
}

func errCodeEqual(t *testing.T, expected string, actual error) {
	t.Helper()

	assert.Equalf(t, expected, influxdb.ErrorCode(actual), "err: %s", actual)
}

func seedEntities(t *testing.T, store pkger.Store, first pkger.Stack, rest ...pkger.Stack) {
	t.Helper()

	for _, st := range append(rest, first) {
		err := store.CreateStack(context.Background(), st)
		require.NoError(t, err)
	}
}
