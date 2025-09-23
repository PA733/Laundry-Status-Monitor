# Laundry Status Monitoring Backend

## Project Overview

This project is a backend service for monitoring the status of laundry machines in university dormitories. It scrapes data from the official laundry service page, stores it, and exposes it through a RESTful API. This allows for the development of third-party applications, such as mobile apps or widgets, to provide students with real-time laundry availability.

## Features

*   **Status Scraping:** Periodically scrapes the laundry service website to get the latest status of all machines.
*   **Historical Data:** Stores historical status data in a database for potential future analysis.
*   **RESTful API:** Provides simple API endpoints to query laundry status.
*   **Dockerized:** The entire application is containerized with Docker for easy setup and deployment.
*   **Configurable:** Key parameters like scraping interval and database credentials can be configured via a `config.yaml` file.

## Architecture

The application uses an event-driven architecture to decouple database transactions from the notification system. This ensures that updates to the database are fast and reliable, while notifications are handled asynchronously.

When the scraper detects changes in machine status, it writes the updates to the database. Instead of directly sending notifications within the same transaction, the store identifies which machines have become idle and returns their IDs. The scraper then dispatches a notification job for each of these machines.

### Notification Worker Pool

A dedicated worker pool, defined in [`internal/notification/worker.go`](internal/notification/worker.go:29), handles the asynchronous sending of push notifications. These workers listen for jobs, fetch the necessary subscription details for a given machine, and send the notifications.

This approach helps manage concurrency, prevents database connection exhaustion by limiting the number of simultaneous notification tasks, and makes the system more resilient to notification delivery failures.

### Configuration

The number of concurrent notification workers can be configured in the [`config/config.yaml`](config/config.yaml:45) file:

```yaml
worker_pool:
  size: 4
```

The `size` parameter controls how many notifications can be sent in parallel. Increasing this value can improve notification throughput but will also increase resource consumption (CPU and database connections).

## Getting Started

### Prerequisites

*   [Docker](https://www.docker.com/get-started)
*   [Docker Compose](https://docs.docker.com/compose/install/)

### Installation & Running

1.  **Clone the repository:**
    ```bash
    git clone <repository-url>
    cd status_backend_hapness_life
    ```

2.  **Configure the application:**
    Copy the example configuration file and modify it if necessary.
    ```bash
    cp config/config.example.yaml config/config.yaml
    ```
    See the [Configuration](#configuration) section for details on the available options.

3.  **Run with Docker Compose:**
    This command will build the Go application, pull the PostgreSQL image, and start both services.
    ```bash
    docker-compose up --build
    ```

The API will be available at `http://localhost:8080`.

## Configuration

The application is configured using the [`config/config.yaml`](config/config.yaml:1) file.

| Parameter          | Description                                                              | Default Value |
| ------------------ | ------------------------------------------------------------------------ | ------------- |
| `server.port`      | The port on which the API server will listen.                            | `8080`        |
| `scraper.interval` | The interval in seconds at which the scraper fetches new data.           | `300`         |
| `scraper.url`      | The URL of the laundry service page to scrape.                           |               |
| `db.host`          | The hostname of the PostgreSQL database.                                 | `db`          |
| `db.port`          | The port of the PostgreSQL database.                                     | `5432`        |
| `db.user`          | The username for the PostgreSQL database.                                | `postgres`    |
| `db.password`      | The password for the PostgreSQL database.                                | `postgres`    |
| `db.dbname`        | The name of the database to use.                                         | `laundry`     |

## API Endpoints

### 1. Get All Dormitories

*   **Description:** Retrieves a list of all dormitories and their associated laundry rooms.
*   **Path:** `/dorms`
*   **Method:** `GET`
*   **Query Parameters:** None
*   **Example Response:**
    ```json
    [
      {
        "ID": 1,
        "Name": "Dormitory A",
        "Location": "North Campus"
      },
      {
        "ID": 2,
        "Name": "Dormitory B",
        "Location": "South Campus"
      }
    ]
    ```

### 2. Get Laundry Status

*   **Description:** Retrieves the current status of all laundry machines, optionally filtered by dormitory ID.
*   **Path:** `/status`
*   **Method:** `GET`
*   **Query Parameters:**
    *   `dorm_id` (optional, integer): The ID of the dormitory to filter by.
*   **Example Response (without filter):**
    ```json
    [
      {
        "MachineID": 101,
        "DormID": 1,
        "Status": "available",
        "UpdatedAt": "2025-09-01T03:30:00Z"
      },
      {
        "MachineID": 102,
        "DormID": 1,
        "Status": "in_use",
        "UpdatedAt": "2025-09-01T03:28:15Z"
      }
    ]
    ```
*   **Example Response (with `?dorm_id=1`):**
    ```json
    [
      {
        "MachineID": 101,
        "DormID": 1,
        "Status": "available",
        "UpdatedAt": "2025-09-01T03:30:00Z"
      },
      {
        "MachineID": 102,
        "DormID": 1,
        "Status": "in_use",
        "UpdatedAt": "2025-09-01T03:28:15Z"
      }
    ]

### 3. Create or Update Subscription

*   **Description:** Creates or replaces a user's subscription for a dorm. This is an idempotent action.
*   **Path:** `/subscriptions`
*   **Method:** `PUT`
*   **Request Body:**
    ```json
    {
      "user_id": "user123",
      "dorm_id": 1
    }
    ```
*   **Success Response:**
    *   **Code:** `204 No Content`
*   **Error Response:**
    *   **Code:** `400 Bad Request`
    *   **Content:**
        ```json
        {
          "error": "Invalid request body"
        }
        ```