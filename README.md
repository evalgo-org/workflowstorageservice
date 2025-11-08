# Workflow Storage Service

S3-backed persistent storage service for workflow definitions and execution results via Schema.org actions.

## Overview

Workflow Storage Service provides semantic and REST interfaces for storing and retrieving workflow data. It uses S3-compatible object storage (Hetzner S3, AWS S3) as the backend and is part of the EVE (Evalgo Virtual Environment) semantic service ecosystem.

## Features

- **Workflow Persistence**: Store and retrieve workflow definitions and results
- **S3 Backend**: Supports AWS S3, Hetzner S3, and S3-compatible storage
- **Dual Interface**: Both semantic actions and REST endpoints
- **File Output Support**: Save results directly to filesystem
- **State Tracking**: Built-in operation state management
- **Auto-Discovery**: Automatic registry service registration
- **API Key Protection**: Optional authentication via API keys

## Architecture

```
REST Endpoints → JSON-LD Conversion → Semantic Action Handler → S3 Storage
```

The service uses a thin REST adapter pattern where all REST endpoints convert requests to Schema.org JSON-LD actions and delegate to the semantic handler.

## Installation

```bash
# Build the service
go build -o workflowstorageservice ./cmd/workflowstorageservice

# Or using task
task build
```

## Configuration

Environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | `8094` |
| `WORKFLOW_STORAGE_API_KEY` | API key for endpoint protection | (optional) |
| `HETZNER_S3_BUCKET` | S3 bucket name | `px-semantic` |
| `HETZNER_S3_ENDPOINT` | S3 endpoint URL | (required) |
| `HETZNER_S3_ACCESS_KEY` | S3 access key | (required) |
| `HETZNER_S3_SECRET_KEY` | S3 secret key | (required) |
| `REGISTRYSERVICE_API_URL` | Registry service URL | (optional) |

## Usage

### Start the service

```bash
export WORKFLOW_STORAGE_API_KEY=your-secret-key
export HETZNER_S3_BUCKET=my-bucket
export HETZNER_S3_ENDPOINT=https://s3.eu-central-1.amazonaws.com
export HETZNER_S3_ACCESS_KEY=your-access-key
export HETZNER_S3_SECRET_KEY=your-secret-key
export PORT=8094
./workflowstorageservice
```

### Health check

```bash
curl http://localhost:8094/health
```

### Service documentation

```bash
curl http://localhost:8094/v1/api/docs
```

## API Reference

### Semantic Action Endpoint (Primary Interface)

**POST** `/v1/api/semantic/action`

Accepts Schema.org JSON-LD actions for storage operations.

#### Supported Actions

##### CreateAction - Store Workflow

```json
{
  "@context": "https://schema.org",
  "@type": "CreateAction",
  "identifier": "my-workflow-001",
  "object": {
    "@type": "DigitalDocument",
    "text": "{\"workflow\": \"definition\"}",
    "encodingFormat": "application/json"
  }
}
```

Response includes S3 location:

```json
{
  "@context": "https://schema.org",
  "@type": "CreateAction",
  "actionStatus": "CompletedActionStatus",
  "result": {
    "@type": "DataDownload",
    "contentUrl": "s3://bucket/workflow-results/default/my-workflow-001.json",
    "encodingFormat": "application/json",
    "contentSize": 1234
  }
}
```

##### RetrieveAction - Fetch Workflow

```json
{
  "@context": "https://schema.org",
  "@type": "RetrieveAction",
  "identifier": "my-workflow-001",
  "object": {
    "@type": "DigitalDocument",
    "contentUrl": "s3://bucket/workflow-results/default/my-workflow-001.json"
  }
}
```

With file output:

```json
{
  "@context": "https://schema.org",
  "@type": "RetrieveAction",
  "identifier": "my-workflow-001",
  "object": {
    "@type": "DigitalDocument",
    "contentUrl": "s3://bucket/workflow-results/default/my-workflow-001.json"
  },
  "outputFile": "/tmp/my-result.json"
}
```

##### UpdateAction - Update Workflow

```json
{
  "@context": "https://schema.org",
  "@type": "UpdateAction",
  "identifier": "my-workflow-001",
  "object": {
    "@type": "DigitalDocument",
    "text": "{\"updated\": \"data\"}",
    "encodingFormat": "application/json"
  }
}
```

##### DeleteAction - Remove Workflow

```json
{
  "@context": "https://schema.org",
  "@type": "DeleteAction",
  "identifier": "my-workflow-001",
  "object": {
    "@type": "DigitalDocument",
    "contentUrl": "s3://bucket/workflow-results/default/my-workflow-001.json"
  }
}
```

### REST Endpoints (Convenience Interface)

All REST endpoints convert to semantic actions internally.

#### Store Workflow

**POST** `/v1/api/workflows`

```bash
curl -X POST http://localhost:8094/v1/api/workflows \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-key" \
  -d '{
    "id": "my-workflow-001",
    "definition": {
      "name": "Test Workflow",
      "steps": []
    },
    "format": "application/json"
  }'
```

#### Retrieve Workflow

**GET** `/v1/api/workflows/:id`

```bash
curl http://localhost:8094/v1/api/workflows/my-workflow-001 \
  -H "X-API-Key: your-secret-key"
```

Query parameters:
- `bucket`: Override default S3 bucket

#### Update Workflow

**PUT** `/v1/api/workflows/:id`

```bash
curl -X PUT http://localhost:8094/v1/api/workflows/my-workflow-001 \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret-key" \
  -d '{
    "definition": {
      "name": "Updated Workflow",
      "steps": []
    }
  }'
```

#### Delete Workflow

**DELETE** `/v1/api/workflows/:id`

```bash
curl -X DELETE http://localhost:8094/v1/api/workflows/my-workflow-001 \
  -H "X-API-Key: your-secret-key"
```

### Legacy Endpoints

The service also supports legacy endpoints for backward compatibility:

- **POST** `/v1/api/store` - Store data
- **GET** `/v1/api/fetch/:key` - Fetch data by key

## State Tracking

The service includes built-in state management for all operations:

```bash
# List all tracked operations
curl http://localhost:8094/v1/api/state

# Get specific operation details
curl http://localhost:8094/v1/api/state/{operation-id}

# Get state statistics
curl http://localhost:8094/v1/api/state/stats
```

## S3 Storage Structure

Workflows are stored in S3 with the following structure:

```
s3://bucket/
└── workflow-results/
    └── {workflow-id}/
        └── {action-id}.json
```

Example:
```
s3://px-semantic/workflow-results/default/my-workflow-001.json
```

## Integration with EVE Ecosystem

### Registry Service

The service automatically registers with the EVE registry service if `REGISTRYSERVICE_API_URL` is configured.

### Workflow Orchestration

Use with the `when` workflow scheduler for persistent workflow execution:

```json
{
  "@context": "https://schema.org",
  "@type": "ItemList",
  "itemListElement": [
    {
      "@type": "CreateAction",
      "identifier": "step-1-result",
      "object": {
        "@type": "DigitalDocument",
        "text": "{\"result\": \"data\"}"
      }
    }
  ]
}
```

## Development

### Project Structure

```
workflowstorageservice/
├── cmd/workflowstorageservice/
│   ├── main.go           # Service entry point
│   ├── rest_handlers.go  # REST endpoint handlers
│   ├── semantic_api.go   # Semantic action handlers
│   └── storage.go        # Legacy storage handlers
```

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o workflowstorageservice ./cmd/workflowstorageservice
```

## License

Apache License 2.0 - See LICENSE file for details.

Copyright 2025 evalgo.org

## Links

- [EVE Documentation](../when/docs/)
- [REST Endpoint Design](../when/REST_ENDPOINT_DESIGN.md)
- [Schema.org Actions](https://schema.org/Action)
