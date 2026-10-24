# Spec Delta

## Purpose

Defines the safe reviewer-facing web surface for understanding the LeeTKOB publication gate and API availability.

## ADDED Requirements

### Requirement: Reviewer dashboard shell
The system SHALL provide a responsive Next.js App Router page that presents publication control, the current lab, pending review count, a review queue, and system checks in a clear hierarchy.

#### Scenario: Desktop reviewer view
- **WHEN** a reviewer opens the web application on a desktop viewport
- **THEN** the publication gate is the primary focus, the review queue remains scannable, and system checks occupy a secondary rail

#### Scenario: Narrow reviewer view
- **WHEN** the viewport cannot support two columns
- **THEN** the status rail moves into the document flow without horizontal overflow or loss of controls

### Requirement: Publication gate presentation
The dashboard SHALL communicate that publication is blocked whenever the displayed pending review count is greater than zero and SHALL keep the publish control disabled in that state.

#### Scenario: Pending reviews are shown
- **WHEN** the current lab has one or more pending reviews
- **THEN** the page shows `Publication blocked`, explains that every review must be resolved, reports the count, and disables `Publish lab`

### Requirement: Query-managed API health
The dashboard SHALL use TanStack Query to request `GET /health`, cache the result, and represent loading, healthy, and failure states without blocking the rest of the page.

#### Scenario: API is healthy
- **WHEN** the health request succeeds
- **THEN** the page identifies the API as connected and displays no problem detail

#### Scenario: API cannot be reached
- **WHEN** the health request fails
- **THEN** the page shows a concise failure message and a retry control while retaining the publication and review context

### Requirement: Safe dashboard data
The PA#2 dashboard SHALL use clearly identified demonstration review rows and SHALL NOT request or render student identity, source code, model credentials, or review evidence.

#### Scenario: Reviewer inspects page content
- **WHEN** the dashboard is rendered in any query state
- **THEN** no protected student data, source code, credentials, or cross-class identifiers are present
