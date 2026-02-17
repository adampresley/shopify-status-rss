package main

import (
	"context"
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/adampresley/httphelpers/responses"
	"github.com/adampresley/mux"
	"github.com/glebarez/sqlite"
	"github.com/hanagantig/cron"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	Version string = "development"

	config *Config
	db     *gorm.DB
)

/*
*******************************************************
Database models
*******************************************************
*/
type LastStatus struct {
	ID             uint      `gorm:"primaryKey"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LastStatusHash string    `json:"lastStatusHash"`
}

type Service struct {
	gorm.Model
	ServiceName string `json:"serviceName"`
}

type Status struct {
	gorm.Model
	Status    string `json:"status"`
	ClassName string `json:"className"`
	IsError   bool   `json:"isError"`
}

type ServiceStatus struct {
	gorm.Model
	StatusID  uint    `json:"statusId"`
	Status    Status  `json:"-"`
	ServiceID uint    `json:"serviceId"`
	Service   Service `json:"-"`
}

type Feed struct {
	ID        uint           `gorm:"primarykey" xml:"-"`
	CreatedAt time.Time      `xml:"-"`
	UpdatedAt time.Time      `xml:"-"`
	DeletedAt gorm.DeletedAt `gorm:"index" xml:"-"`

	Title       string    `json:"title" xml:"title"`
	PubDate     time.Time `json:"pubDate" xml:"pubDate"`
	Description string    `json:"description" xml:"description"`
}

type CronLock struct {
	gorm.Model
	Key string
}

/*
*******************************************************
App models
*******************************************************
*/
type ParsedStatus struct {
	Service *Service
	Status  *Status
}

type ParsedStatusCollection []ParsedStatus

type RssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	AtomNS  string     `xml:"xmlns:atom,attr"`
	Channel RssChannel `xml:"channel"`
}

type RssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language"`
	Generator   string    `xml:"generator"`
	Items       []RssItem `xml:"item"`
}

type RssItem struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	PubDate     time.Time `xml:"pubDate"`
}

/*
*******************************************************
Main
*******************************************************
*/
func main() {
	var (
		err      error
		dialect  gorm.Dialector
		statuses []*Status
		services []*Service
	)

	config = LoadConfig()
	setupLogging()
	shutdownCtx, stopApp := context.WithCancel(context.Background())

	/*
	 * Database
	 */
	if strings.HasPrefix(config.DSN, "file:") {
		dialect = sqlite.Open(config.DSN)
	} else if strings.HasPrefix(config.DSN, "postgres:") || strings.HasPrefix(config.DSN, "postgresql:") {
		dialect = postgres.Open(config.DSN)
	} else {
		panic("Unsupported database dialect")
	}

	if db, err = gorm.Open(dialect, &gorm.Config{}); err != nil {
		slog.Error("error connecting to database", "error", err)
		os.Exit(1)
	}

	slog.Info("Database connection established. Running migrations...")

	db.AutoMigrate(
		&Service{}, &Status{}, &ServiceStatus{},
		&Feed{}, &LastStatus{}, &CronLock{},
	)

	if statuses, err = queryStatuses(); err != nil {
		panic("error querying statuses: " + err.Error())
	}

	if services, err = queryServices(); err != nil {
		panic("error querying services: " + err.Error())
	}

	routes := []mux.Route{
		{Path: "GET /status.rss", HandlerFunc: statusRssHandler()},
	}

	muxer := mux.Setup(
		config,
		routes,
		shutdownCtx,
		stopApp,

		mux.WithDebug(Version == "development"),
		mux.WithMiddlewares(
			func(h http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					slog.Info("request", "method", r.Method, "path", r.URL.Path)
					h.ServeHTTP(w, r)
				})
			},
		),
	)

	postgresLocker := &PostgresLocker{DB: db}

	c := cron.New(
		cron.WithLocks(postgresLocker),
	)

	c.AddFunc(config.CronSchedule, "check-status", func() {
		cronJob(services, statuses)
	})

	cronJob(services, statuses)
	c.Start()

	slog.Info("server started", "host", config.Host, "schedule", config.CronSchedule, "statusPage", config.StatusPageURL, "version", Version)
	muxer.Start()
}

func setupLogging() {
	var (
		logger *slog.Logger
	)

	level := slog.LevelInfo

	switch strings.ToLower(config.LogLevel) {
	case "debug":
		level = slog.LevelDebug

	case "error":
		level = slog.LevelError

	default:
		level = slog.LevelInfo
	}

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}).WithAttrs([]slog.Attr{
		slog.String("version", Version),
	})

	logger = slog.New(h)
	slog.SetDefault(logger)
}

func cronJob(services []*Service, statuses []*Status) {
	var (
		err        error
		doc        *goquery.Document
		states     = ParsedStatusCollection{}
		lastStatus *LastStatus
		rssItem    RssItem
	)

	if doc, err = grabStatusPage(config.StatusPageURL); err != nil {
		slog.Error("error grabbing status page", "error", err)
		return
	}

	if states, err = parsePageStatuses(doc, services, statuses); err != nil {
		slog.Error("error parsing page statuses", "error", err)
		return
	}

	hash := generateStatusHash(states)

	if lastStatus, err = queryLastStatus(); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("error querying last status", "error", err)
		return
	}

	/*
	 * We have no records. Make one
	 */
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err = insertLastStatus(hash); err != nil {
			slog.Error("error creating last status record", "error", err)
		}

		if states.HasErrors() {
			rssItem = generateErrorFeedItem(states)
		} else {
			rssItem = generateOperationalFeedItem(states)
		}

		if err = insertRssItem(rssItem); err != nil {
			slog.Error("error inserting RSS item", "error", err)
		}

		return
	}

	/*
	 * If we do have a record, check to see if the hash has changed.
	 * If it has, did it flip to an error state, or did it flip back to a normal state?
	 */
	if lastStatus.LastStatusHash == hash {
		slog.Info("no changes detected in status page")
		return
	}

	if err = updateLastStatus(hash); err != nil {
		slog.Error("error updating last status record", "error", err)
		return
	}

	if states.HasErrors() {
		slog.Info("status page has errors. writing to feed", "hash", hash)
		rssItem = generateErrorFeedItem(states)
	} else {
		slog.Info("status page is back to normal. writing to feed", "hash", hash)
		rssItem = generateOperationalFeedItem(states)
	}

	if err = insertRssItem(rssItem); err != nil {
		slog.Error("error inserting RSS item", "error", err)
	}
}

/*
*******************************************************
Model functions
*******************************************************
*/

func (psc ParsedStatusCollection) HasErrors() bool {
	for _, status := range psc {
		if status.Status.IsError {
			return true
		}
	}

	return false
}

/*
*******************************************************
Handlers
*******************************************************
*/
func statusRssHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			err  error
			feed []*Feed
			b    []byte
		)

		if feed, err = queryFeed(10); err != nil {
			responses.TextInternalServerError(w, "An unexpected error occurred while querying the feed")
			return
		}

		result := RssFeed{
			Version: "2.0",
			AtomNS:  "http://www.w3.org/2005/Atom",
			Channel: RssChannel{
				Title:       "Shopify Services Status",
				Link:        config.StatusPageURL,
				Description: "Providing the current status of Shopify services through RSS!",
				Language:    "en",
				Generator:   "shopify-status-rss by Adam Presley",
				Items:       []RssItem{},
			},
		}

		for _, f := range feed {
			result.Channel.Items = append(result.Channel.Items, RssItem{
				Title:       f.Title,
				Link:        config.StatusPageURL,
				Description: f.Description,
				PubDate:     f.PubDate,
			})
		}

		if b, err = xml.Marshal(result); err != nil {
			responses.TextInternalServerError(w, "An unexpected error occurred while marshalling the RSS feed")
			return
		}

		b = append([]byte(xml.Header), b...)
		responses.Bytes(w, http.StatusOK, "application/xml", b)
	}
}

/*
*******************************************************
General functions
*******************************************************
*/
func getContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Second*10)
}

func grabStatusPage(url string) (*goquery.Document, error) {
	var (
		err      error
		response *http.Response
		doc      *goquery.Document
	)

	if response, err = http.Get(url); err != nil {
		return doc, fmt.Errorf("error fetching status page '%s': %w", url, err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return doc, fmt.Errorf("status page '%s' returned status code %d", url, response.StatusCode)
	}

	if doc, err = goquery.NewDocumentFromReader(response.Body); err != nil {
		return doc, fmt.Errorf("error parsing status page '%s': %w", url, err)
	}

	return doc, nil
}

func generateErrorFeedItem(states ParsedStatusCollection) RssItem {
	var (
		description             = strings.Builder{}
		servicesWithIssuesCount = 0
	)

	fmt.Fprintf(&description, `<h2>Shopify Reports Issues</h2>`)
	fmt.Fprintf(&description, `<p>The Shopify status page may be reporting issues. The 
		following services are experiencing problems:</p>`)
	fmt.Fprintf(&description, `<ul>`)

	for _, status := range states {
		if status.Status.IsError {
			fmt.Fprintf(&description, `<li>%s - %s</li>`, status.Service.ServiceName, status.Status.Status)
			servicesWithIssuesCount++
		}
	}

	fmt.Fprintf(&description, `</ul>`)

	result := RssItem{
		Title:       fmt.Sprintf("%d services reporting potential issues", servicesWithIssuesCount),
		Link:        "https://my.shopifystatus.com",
		Description: description.String(),
		PubDate:     time.Now().UTC(),
	}

	return result
}

func generateOperationalFeedItem(states ParsedStatusCollection) RssItem {
	var (
		description = strings.Builder{}
	)

	fmt.Fprintf(&description, `<h2>Shopify Is Operational</h2>`)
	fmt.Fprintf(&description, `<p>The Shopify status page shows that all services appear to be operational.</p>`)
	fmt.Fprintf(&description, `<ul>`)

	for _, status := range states {
		fmt.Fprintf(&description, `<li>%s - %s</li>`, status.Service.ServiceName, status.Status.Status)

		if status.Status.IsError {
		}
	}

	fmt.Fprintf(&description, `</ul>`)

	result := RssItem{
		Title:       "All services appear to be operational",
		Link:        "https://my.shopifystatus.com",
		Description: description.String(),
		PubDate:     time.Now().UTC(),
	}

	return result
}

func generateStatusHash(parsedStatuses []ParsedStatus) string {
	hasher := sha256.New()

	for _, status := range parsedStatuses {
		fmt.Fprintf(hasher, "%s:%s", status.Service.ServiceName, status.Status.ClassName)
	}

	result := hasher.Sum(nil)
	return fmt.Sprintf("%x", result)
}

func parsePageStatuses(doc *goquery.Document, services []*Service, statuses []*Status) (ParsedStatusCollection, error) {
	var (
		result = ParsedStatusCollection{}
	)

	wantServiceCount := len(services)
	gotCount := 0

	doc.Find("div.flex-col > p").Each(func(i int, s *goquery.Selection) {
		for _, service := range services {
			if service.ServiceName == s.Text() {
				gotCount++
				result = append(result, ParsedStatus{Service: service})
				return
			}
		}
	})

	if gotCount != wantServiceCount {
		return result, fmt.Errorf("the number of services on the page does not match the number of services in the database. something has changed")
	}

	gotCount = 0

	doc.Find("div.flex-col i").Each(func(i int, s *goquery.Selection) {
		for _, status := range statuses {
			if s.HasClass(status.ClassName) {
				if i < wantServiceCount {
					gotCount++
					result[i].Status = status
					return
				}
			}
		}
	})

	if gotCount != wantServiceCount {
		return result, fmt.Errorf("the number of status icons on the page does not match the number of statuses in the database. something has changed")
	}

	slices.SortStableFunc(result, func(a, b ParsedStatus) int {
		return strings.Compare(a.Service.ServiceName, b.Service.ServiceName)
	})

	return result, nil
}

/*
*******************************************************
Data functions
*******************************************************
*/
func queryFeed(limit int) ([]*Feed, error) {
	ctx, cancel := getContext()
	defer cancel()

	tx := gorm.G[*Feed](db).Order("created_at DESC")

	if limit > 0 {
		tx = tx.Limit(limit)
	}

	return tx.Find(ctx)
}

func queryLastStatus() (*LastStatus, error) {
	ctx, cancel := getContext()
	defer cancel()

	return gorm.G[*LastStatus](db).First(ctx)
}

func queryStatuses() ([]*Status, error) {
	var (
		err      error
		statuses []*Status
	)

	ctx, cancel := getContext()
	defer cancel()

	if statuses, err = gorm.G[*Status](db).Find(ctx); err != nil {
		return statuses, fmt.Errorf("error querying statuses: %w", err)
	}

	return statuses, nil
}

func queryServices() ([]*Service, error) {
	var (
		err      error
		services []*Service
	)

	ctx, cancel := getContext()
	defer cancel()

	if services, err = gorm.G[*Service](db).Find(ctx); err != nil {
		return services, fmt.Errorf("error querying services: %w", err)
	}

	return services, nil
}

func insertLastStatus(hash string) error {
	ctx, cancel := getContext()
	defer cancel()

	return gorm.G[LastStatus](db).Create(ctx, &LastStatus{
		ID:             1,
		UpdatedAt:      time.Now(),
		LastStatusHash: hash,
	})
}

func updateLastStatus(hash string) error {
	ctx, cancel := getContext()
	defer cancel()

	_, err := gorm.G[LastStatus](db).Where("id=1").Update(ctx, "last_status_hash", hash)
	return err
}

func insertRssItem(item RssItem) error {
	ctx, cancel := getContext()
	defer cancel()

	feedItem := Feed{
		Title:       item.Title,
		PubDate:     item.PubDate,
		Description: item.Description,
	}

	return gorm.G[Feed](db).Create(ctx, &feedItem)
}
