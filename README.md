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

The system is designed with a modular architecture, consisting of a scraper, a database, and an API server.

*   **Scraper:** A Go service responsible for fetching and parsing the HTML from the laundry service page.
*   **Database:** A PostgreSQL database to store dormitory, machine, and status information.
*   **API Server:** A Go service providing HTTP endpoints to access the laundry data.

For a more detailed breakdown of the architecture, please refer to [`ARCHITECTURE.md`](ARCHITECTURE.md).

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