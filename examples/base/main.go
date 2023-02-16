package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/forms"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/models/schema"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cobra"
)

func main() {
	app := pocketbase.New()

	// ---------------------------------------------------------------
	// Optional plugin flags:
	// ---------------------------------------------------------------

	var migrationsDir string
	app.RootCmd.PersistentFlags().StringVar(
		&migrationsDir,
		"migrationsDir",
		"",
		"the directory with the user defined migrations",
	)

	var automigrate bool
	app.RootCmd.PersistentFlags().BoolVar(
		&automigrate,
		"automigrate",
		true,
		"enable/disable auto migrations",
	)

	var publicDir string
	app.RootCmd.PersistentFlags().StringVar(
		&publicDir,
		"publicDir",
		defaultPublicDir(),
		"the directory to serve static files",
	)

	var indexFallback bool
	app.RootCmd.PersistentFlags().BoolVar(
		&indexFallback,
		"indexFallback",
		true,
		"fallback the request to index.html on missing static path (eg. when pretty urls are used with SPA)",
	)

	app.RootCmd.ParseFlags(os.Args[1:])

	// ---------------------------------------------------------------
	// Plugins and hooks:
	// ---------------------------------------------------------------

	// load js pb_migrations
	jsvm.MustRegisterMigrations(app, &jsvm.MigrationsOptions{
		Dir: migrationsDir,
	})

	// migrate command (with js templates)
	migratecmd.MustRegister(app, app.RootCmd, &migratecmd.Options{
		TemplateLang: migratecmd.TemplateLangJS,
		Automigrate:  automigrate,
		Dir:          migrationsDir,
	})

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		// serves static files from the provided public dir (if exists)
		e.Router.GET("/*", apis.StaticDirectoryHandler(os.DirFS(publicDir), indexFallback))
		return nil
	})

	app.OnRecordBeforeCreateRequest().Add(func(e *core.RecordCreateEvent) error {

		c := e.HttpContext
		record := e.Record
		collection := record.Collection()
		switch collection.Name {
		case "posts":
			admin, _ := c.Get(apis.ContextAdminKey).(*models.Admin)
			authRecord, _ := c.Get(apis.ContextAuthRecordKey).(*models.Record)
			// check if the user is member of project
			fmt.Println("")
			fmt.Println("record", record.Get("projects"))
			fmt.Println("AuthRecord", authRecord)
			if admin != nil {
				return nil
			}

			return apis.NewForbiddenError("The authorized record model is not allowed to perform this action.", nil)
			// return hook.StopPropagation
		}
		return nil
	})

	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "form",
		Short: "Setup database for testing purposes",
		Run: func(command *cobra.Command, args []string) {

			//Fetch the users collection
			usersCollection, err := app.Dao().FindCollectionByNameOrId("users")
			if err != nil {
				fmt.Println(usersCollection)
				log.Fatal(err)
			}

			//Delete all existing collections for reformation
			die(deleteCollections(app, "posts", "memberOf", "projects"))
			fmt.Println("Cleared existing all collections.")

			// Create collections
			projectCollection, err := makeStandardCollection(app, "projects", []*schema.SchemaField{
				{
					Name:     "name",
					Type:     schema.FieldTypeText,
					Required: true,
					Unique:   true,
					Options: &schema.TextOptions{
						Max: types.Pointer(50),
					},
				}, {
					Name:     "description",
					Type:     schema.FieldTypeEditor,
					Required: true,
					Unique:   false,
				}, {
					Name:     "thumbnail",
					Type:     schema.FieldTypeFile,
					Required: false,
					Unique:   false,
					Options: &schema.FileOptions{
						MaxSelect: 1,
						MaxSize:   250_000,
						MimeTypes: []string{"image/jpeg", "image/png", "image/svg+xml", "image/gif", "image/webp"},
					},
				},
			})
			die(err)

			memberOfCollection, err := makeStandardCollection(app, "memberOf", []*schema.SchemaField{
				{
					Name:     "user_",
					Type:     schema.FieldTypeRelation,
					Required: true,
					Options: &schema.RelationOptions{
						MaxSelect:     types.Pointer(1),
						CollectionId:  usersCollection.Id,
						CascadeDelete: true,
					},
				}, {
					Name:     "_project",
					Type:     schema.FieldTypeRelation,
					Required: true,
					Options: &schema.RelationOptions{
						MaxSelect:     types.Pointer(1),
						CollectionId:  projectCollection.Id,
						CascadeDelete: true,
					},
				}, {
					Name:     "role",
					Type:     schema.FieldTypeText,
					Required: false,
					Unique:   false,
					Options: &schema.TextOptions{
						Max: types.Pointer(50),
					},
				}, {
					Name:     "contacts",
					Type:     schema.FieldTypeJson,
					Required: false,
					Unique:   false,
				},
			})
			die(err)
			_ = memberOfCollection
			postCollection, err := makeStandardCollection(app, "posts", []*schema.SchemaField{
				{
					Name:     "projects",
					Type:     schema.FieldTypeRelation,
					Required: false,
					Unique:   false,
					Options: &schema.RelationOptions{
						MaxSelect:     types.Pointer(8),
						CollectionId:  projectCollection.Id,
						CascadeDelete: true,
					},
				},
				{
					Name:     "authors",
					Type:     schema.FieldTypeRelation,
					Required: false,
					Unique:   false,
					Options: &schema.RelationOptions{
						MaxSelect:     types.Pointer(8),
						CollectionId:  usersCollection.Id,
						CascadeDelete: true,
					},
				},
				{
					Name:     "type",
					Type:     schema.FieldTypeSelect,
					Required: true,
					Unique:   false,
					Options: &schema.SelectOptions{
						MaxSelect: 1,
						Values:    []string{"ProjectUpdate", "Standalone", "Comment"},
					},
				}, {
					Name:     "metadata",
					Type:     schema.FieldTypeJson,
					Required: false,
					Unique:   false,
					Options:  &schema.JsonOptions{},
				}, {
					Name:     "content",
					Type:     schema.FieldTypeEditor,
					Required: true,
					Unique:   false,
					Options:  &schema.EditorOptions{},
				}, {
					Name:     "files",
					Type:     schema.FieldTypeFile,
					Required: false,
					Unique:   false,
					Options: &schema.FileOptions{
						MaxSelect: 256,
						MaxSize:   500_000,
						MimeTypes: []string{"image/jpeg", "image/png", "image/svg+xml", "image/gif", "image/webp"},
					},
				},
			})
			die(err)

			_ = postCollection
			// {

			// }

			fmt.Println("Creating test records:")

			TEST_USER_COUNT := 10
			MAKE_USERS := false
			users := make([]*models.Record, TEST_USER_COUNT)
			if MAKE_USERS {
				// Test users
				for i := 0; i < TEST_USER_COUNT; i++ {
					record := models.NewRecord(usersCollection)
					form := forms.NewRecordUpsert(app, record)
					username := fmt.Sprintf("TEST_USER_%d", i)
					email := fmt.Sprintf("test%d@gmail.com", i)
					name := fmt.Sprintf("user%d", i)
					password := fmt.Sprintf("testpswd%d", i)

					// fmt.Println(username, email, name, password)
					form.LoadData(map[string]any{
						"username":        username,
						"email":           email,
						"name":            name,
						"password":        password,
						"passwordConfirm": password,
					})
					err := form.Submit()
					die(err)
					users[i] = record
				}
			} else {
				for i := 0; i < TEST_USER_COUNT; i++ {
					username := fmt.Sprintf("TEST_USER_%d", i)
					record, err := app.Dao().FindRecordsByExpr(usersCollection.Id, dbx.HashExp{"username": username})
					die(err)
					if len(record) == 0 {
						log.Fatal(fmt.Sprintf("No user with name $s", username))
					}
					users[i] = record[0]
				}
			}

			// Test project

			projectRecord := models.NewRecord(projectCollection)
			{
				form := forms.NewRecordUpsert(app, projectRecord)
				form.LoadData(map[string]any{
					"name":        "Projective",
					"description": "this",
				})
				err := form.Submit()
				die(err)
			}
			// Test memberships
			{
				record := models.NewRecord(memberOfCollection)
				form := forms.NewRecordUpsert(app, record)
				form.LoadData(map[string]any{
					"user_":    users[1].Id,
					"_project": projectRecord.Id,
					"role":     "role",
					"contacts": "null",
				})

				err := form.Submit()
				die(err)
			}

		},
	})

	app.OnRecordBeforeCreateRequest().Add(func(e *core.RecordCreateEvent) error {
		log.Println(e.Record) // still unsaved
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

/*
*

	Quits if error not null
*/
func die(err error) {
	if err != nil {
		debug.PrintStack()
		log.Fatal(err)
	}
}
func makeStandardCollection(app *pocketbase.PocketBase, name string, fields []*schema.SchemaField) (*models.Collection, error) {
	return makeCollection(app, name, fields, types.Pointer(""), types.Pointer("@request.auth.id != ''"), types.Pointer(""), types.Pointer("@request.auth.id != ''"), nil)
}
func makeCollection(app *pocketbase.PocketBase, name string, fields []*schema.SchemaField,
	listRule *string, viewRule *string, createRule *string, updateRule *string, deleteRule *string) (*models.Collection, error) {
	collection := &models.Collection{}
	form := forms.NewCollectionUpsert(app, collection)
	form.Type = models.CollectionTypeBase
	form.Name = name
	form.ListRule = listRule
	form.ViewRule = viewRule
	form.CreateRule = createRule
	form.UpdateRule = updateRule
	form.DeleteRule = deleteRule
	for _, field := range fields {
		form.Schema.AddField(field)
	}
	err := form.Submit()
	return collection, err
}

func tryDeleteCollection(app *pocketbase.PocketBase, name string) (bool, error) {
	//Delete posts collection
	collection, err := app.Dao().FindCollectionByNameOrId(name)
	if err != nil {
		return false, err
	}
	if collection != nil {
		err := app.Dao().DeleteCollection(collection)
		return err == nil, err
	}
	return false, nil
}
func deleteCollections(app *pocketbase.PocketBase, names ...string) error {
	for _, name := range names {
		_, err := tryDeleteCollection(app, name)
		if err != nil {
			return err
		}
	}
	return nil
}

// type DbFormation struct {
// 	Name         string        `form:"name" json:"name"`
// 	ListRule     *string       `form:"listRule" json:"listRule"`
// 	ViewRule     *string       `form:"viewRule" json:"viewRule"`
// 	CreateRule   *string       `form:"createRule" json:"createRule"`
// 	UpdateRule   *string       `form:"updateRule" json:"updateRule"`
// 	DeleteRule   *string       `form:"deleteRule" json:"deleteRule"`
// 	Options      types.JsonMap `form:"options" json:"options"`
// 	SchemaFields []*schema.SchemaField
// }

// func formDatabase(app *pocketbase.PocketBase, formation []DbFormation) {

// }

// the default pb_public dir location is relative to the executable
func defaultPublicDir() string {
	if strings.HasPrefix(os.Args[0], os.TempDir()) {
		// most likely ran with go run
		return "./pb_public"
	}
	return filepath.Join(os.Args[0], "../pb_public")
}
