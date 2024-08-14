package db

import (
	"bytes"
	"database/sql"
	"fmt"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

const (
	DbPath = "/Users/michaelsageryd/Library/Mobile Documents/iCloud~md~obsidian/Documents/the-vault/.obsidian/plugins/obsidian-sqlite-sync/alfred_sync.db"
	
	TitleKey  = "title"
	TextKey   = "content"
	TagsKey   = "tags"
	NoteIDKey = "path"
	LastModifiedKey = "last_modified"

	RECENT_NOTES = `
		SELECT 
			note.path, 
			note.title,
			CAST(last_modified AS TEXT) as last_modified,
			GROUP_CONCAT(ntag.tag_name) AS tags
		FROM
			note
		LEFT JOIN
			note_tag ntag ON note.path = nTag.note_path
		GROUP BY
			note.path,
			note.title
		ORDER BY
			note.last_modified DESC
		LIMIT 25
	`;

	NOTES_BY_QUERY = `
		SELECT
			note.path, 
			note.title,
			CAST(last_modified AS TEXT) as last_modified,
			GROUP_CONCAT(ntag.tag_name) AS tags
		FROM
				note
		LEFT JOIN
				note_tag ntag ON note.path = nTag.note_path
		WHERE
			note.title LIKE '%'||$1||'%' OR
			note.content_lower LIKE '%'||$1||'%'
		GROUP BY
				note.path,
				note.title
		ORDER BY
				CASE WHEN note.title LIKE '%'||$1||'%' THEN 0 ELSE 1 END,
				note.last_modified DESC
		LIMIT 200;
`

	NOTES_BY_TAGS_AND_QUERY = `
WITH 
	joined AS (
		SELECT
			note.ZUNIQUEIDENTIFIER,
			note.ZTITLE,
			note.ZTEXT,
			note.ZMODIFICATIONDATE,
			tag.ZTITLE AS TAG_TITLE,
			images.ZSEARCHTEXT
		FROM
			ZSFNOTE note
		INNER JOIN
			Z_5TAGS nTag ON note.Z_PK = nTag.Z_5NOTES
		INNER JOIN
			ZSFNOTETAG tag ON nTag.Z_13TAGS = tag.Z_PK
		LEFT JOIN
			ZSFNOTEFILE images ON images.ZNOTE = note.Z_PK
		WHERE
			note.ZARCHIVED = 0
			AND note.ZTRASHED = 0
			AND note.ZTEXT IS NOT NULL
	),
	hasSearchedTags AS (
		{{ .TagIntersection}}
	)
SELECT
	ZUNIQUEIDENTIFIER,
	ZTITLE,
	GROUP_CONCAT(DISTINCT TAG_TITLE) AS TAGS
FROM
	joined
WHERE
	ZUNIQUEIDENTIFIER IN hasSearchedTags 
	AND (
		utflower(ZTITLE) LIKE utflower('%{{ .Text}}%') OR
		utflower(ZTEXT) LIKE utflower('%{{ .Text}}%') OR
		ZSEARCHTEXT LIKE utflower('%{{ .Text}}%')
	)
GROUP BY
	ZUNIQUEIDENTIFIER,
	ZTITLE
ORDER BY
	CASE WHEN utflower(ZTITLE) LIKE utflower('%{{ .Text}}%') THEN 0 ELSE 1 END,
	ZMODIFICATIONDATE DESC
LIMIT 200
`

	TAGS_BY_TITLE = `
		SELECT
				nt.tag_name
		FROM
				note n
		INNER JOIN
				note_tag nt ON n.path = nt.note_path
		WHERE
			nt.tag_name LIKE '%%%s%%' 
		ORDER BY
				n.last_modified DESC
		LIMIT 25
`

	NOTE_TITLE_BY_ID = `
	SELECT
			title
		FROM
			note
		WHERE
			path = '%s'
		ORDER BY
			last_modified DESC
		LIMIT 25;
	`

	NOTE_TEXT_BY_ID = `
		SELECT
			content
		FROM
			note
		WHERE
			path = '%s'
		ORDER BY
			last_modified DESC
		LIMIT 25;
	`
)

type TagQueryArg struct {
	Text            string
	TagIntersection string
}

func TemplateToString(templateStr string, data any) (string, error) {
	var buffer bytes.Buffer
	t := template.Must(template.New("").Parse(templateStr))
	err := t.Execute(&buffer, data)
	return buffer.String(), errors.WithStack(err)
}

type Note map[string]string

func Expanduser(path string) string {
	usr, _ := user.Current()
	dir := usr.HomeDir
	if path[:2] == "~/" {
		path = filepath.Join(dir, path[2:])
	}
	return path
}

type LiteDB struct {
	db *sql.DB
}

func NewLiteDB(path string) (LiteDB, error) {
	sql.Register("sqlite3_custom", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.RegisterFunc("utflower", utfLower, true)
		},
	})

	db, err := sql.Open("sqlite3_custom", path)
	litedb := LiteDB{db}
	return litedb, err
}

 func utfLower(s string) string {
 	return strings.ToLower(s)
 }

func NewBearDB() (LiteDB, error) {
	path := Expanduser(DbPath)
	litedb, err := NewLiteDB(path)
	return litedb, err
}

func (litedb LiteDB) Query(q string, args ...interface{}) ([]Note, error) {
	results := []Note{}
	rows, err := litedb.db.Query(q, args...)
	if err != nil {
		return results, errors.WithStack(err)
	}

	cols, err := rows.Columns()
	if err != nil {
		return results, errors.WithStack(err)
	}

	for rows.Next() {
		m := Note{}
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}
		if err := rows.Scan(columnPointers...); err != nil {
			return results, errors.WithStack(err)
		}
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			s, ok := (*val).(string)
			if ok {
				m[colName] = s
			} else {
				m[colName] = ""
			}
		}
		results = append(results, m)
	}
	err = rows.Close()
	if err != nil {
		return results, errors.WithStack(err)
	}
	err = rows.Err()
	if err != nil {
		return results, errors.WithStack(err)
	}
	return results, errors.WithStack(err)
}

func escape(s string) string {
	return strings.Replace(s, "'", "''", -1)
}

func containsOrderedWords(text string, words []string) bool {
	prev := 0
	for _, w := range words {
		i := strings.Index(text, w)
		if i == -1 || i < prev {
			return false
		}
		prev = i
	}
	return true
}

func containsWords(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

func (litedb LiteDB) queryNotesByTextAndTagConjunction(text, tagIntersection string, tags []string) ([]Note, error) {
	tagQueryArg := TagQueryArg{
		Text:            escape(text),
		TagIntersection: tagIntersection,
	}

	query, err := TemplateToString(NOTES_BY_TAGS_AND_QUERY, tagQueryArg)
	if err != nil {
		return nil, err
	}

	return litedb.Query(query)
}

func RemoveTagHashes(tag string) string {
	tag = tag[1:]
	if strings.HasSuffix(tag, "#") {
		tag = tag[:len(tag)-1]
	}
	return tag
}

func (litedb LiteDB) QueryNotesByTextAndTags(text string, tags []string) ([]Note, error) {
	var selectStatements []string
	for _, t := range tags {
		s := fmt.Sprintf("SELECT ZUNIQUEIDENTIFIER FROM joined WHERE utflower(TAG_TITLE) = utflower('%s')", RemoveTagHashes(t))
		selectStatements = append(selectStatements, s)
	}
	tagIntersection := strings.Join(selectStatements, "\nINTERSECT\n")

	wordQuery := func(word string) ([]Note, error) {
		return litedb.queryNotesByTextAndTagConjunction(word, tagIntersection, tags)
	}

	return multiWordQuery(text, wordQuery)
}

func (litedb LiteDB) QueryNotesByText(text string) ([]Note, error) {
	wordQuery := func(word string) ([]Note, error) {
		word = strings.ToLower(word)
		word = escape(word)
		return litedb.Query(NOTES_BY_QUERY, word)
	}
	return multiWordQuery(text, wordQuery)
}

func splitSpacesOrQuoted(s string) []string {
	r := regexp.MustCompile(`([^\s"']+)|"([^"]*)"`)
	matches := r.FindAllStringSubmatch(s, -1)
	var split []string
	for _, v := range matches {
		match := v[1]
		if match == "" {
			match = v[2]
		}
		split = append(split, match)
	}
	return split
}

type noteRecord struct {
	note                 Note
	contains             bool
	containsOrderedWords bool
	containsWords        bool
	originalIndex        int
}

func NewNoteRecord(i int, note Note, lowerText string) *noteRecord {
	title := strings.ToLower(note[TitleKey])
	words := strings.Split(lowerText, " ")
	record := noteRecord{
		originalIndex:        i,
		note:                 note,
		contains:             strings.Contains(title, lowerText),
		containsOrderedWords: containsOrderedWords(title, words),
		containsWords:        containsWords(title, words),
	}
	return &record
}

func multiWordQuery(text string, wordQuery func(string) ([]Note, error)) ([]Note, error) {
	lowerText := strings.ToLower(text)
	words := splitSpacesOrQuoted(lowerText)

	var records []*noteRecord
	fullMatch := map[string]bool{}
	notes, err := wordQuery(lowerText)
	if err != nil {
		return nil, err
	}
	for i, note := range notes {
		noteId := note[NoteIDKey]
		record := NewNoteRecord(i, note, lowerText)
		record.originalIndex = i
		records = append(records, record)
		fullMatch[noteId] = true
	}

	var multiRecords []*noteRecord
	count := map[string]int{}
	for _, word := range words {
		notes, err := wordQuery(word)
		if err != nil {
			return nil, err
		}

		for i, note := range notes {
			noteId := note[NoteIDKey]
			if count[noteId] == 0 && !fullMatch[noteId] {
				record := NewNoteRecord(i, note, lowerText)
				record.originalIndex = i
				multiRecords = append(multiRecords, record)
			}
			count[noteId]++
		}
	}

	for _, record := range multiRecords {
		if count[record.note[NoteIDKey]] == len(words) || record.containsWords {
			records = append(records, record)
		}
	}

	sort.Slice(records, func(i, j int) bool {
		iRecord := records[i]
		jRecord := records[j]

		if iRecord.contains != jRecord.contains {
			return iRecord.contains
		}

		if iRecord.containsOrderedWords != jRecord.containsOrderedWords {
			return iRecord.containsOrderedWords
		}

		if iRecord.containsWords != jRecord.containsWords {
			return iRecord.containsWords
		}

		return iRecord.originalIndex < jRecord.originalIndex
	})

	var finalRows []Note
	for _, noteRecord := range records {
		finalRows = append(finalRows, noteRecord.note)
	}

	return finalRows, nil
}
