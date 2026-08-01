package portmock

//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.ActivityServicePort -o activity_service_mock.go
//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.GraphVersionWriter -o graph_version_writer_mock.go
//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.JSONLDParserPort -o jsonld_parser_mock.go
//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.MediaStoragePort -o media_storage_mock.go
//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.OutboundDispatcher -o outbound_dispatcher_mock.go
//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.RemoteFetcher -o remote_fetcher_mock.go
//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.StoragePort -o storage_mock.go
//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.StorageAndGraphWriter -o storage_and_graph_writer_mock.go
//go:generate go run github.com/gojuno/minimock/v3/cmd/minimock@v3.4.7 -i sprezz/internal/domain/port.FollowersSyncCache -o followers_sync_cache_mock.go
