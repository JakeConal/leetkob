# review-dashboard Specification

## Purpose

Defines the safe reviewer-facing web surface for understanding the LeeTKOB publication gate and API availability.

## Requirements

### Requirement: Reviewer dashboard shell
The system SHALL provide a responsive Next.js App Router page that presents publication control, the current lab, pending review count, a review queue, and system checks in a clear hierarchy.

#### Scenario: Desktop reviewer view
- **WHEN** a reviewer opens the web application on a desktop viewport
- **THEN** the publication gate is the primary focus, the review queue remains scannable, and system checks occupy a secondary rail

#### Scenario: Narrow reviewer view
- **WHEN** the viewport cannot support two columns
- **THEN** the status rail moves into the document flow without horizontal overflow or loss of controls

### Requirement: Publication gate presentation
The dashboard SHALL present both FAQ-12 preconditions and communicate that publication is blocked whenever the submission deadline has not passed or the displayed pending review count is greater than zero. It SHALL keep the publish control disabled in either state. PA#3 live mode SHALL additionally show processing state and keep production publication unavailable throughout this checkpoint.

#### Scenario: Pending reviews are shown
- **WHEN** the current lab has one or more pending reviews
- **THEN** the page shows `Publication blocked`, explains that every review must be resolved, reports the count, and disables `Publish lab`

#### Scenario: Submission deadline status is shown
- **WHEN** the current lab is displayed
- **THEN** the page exposes whether the submission deadline has passed as a distinct publication check without exposing protected data

#### Scenario: PA3-UI-01 Processing continues
- **WHEN** the live queue has no pending reviews but current judging or checking remains incomplete
- **THEN** the page shows processing as incomplete and does not enable mark publication

### Requirement: Query-managed API health
The dashboard SHALL use TanStack Query to request `GET /health`, cache the result, and represent loading, healthy, and failure states without blocking the rest of the page.

#### Scenario: API is healthy
- **WHEN** the health request succeeds
- **THEN** the page identifies the API as connected and displays no problem detail

#### Scenario: API cannot be reached
- **WHEN** the health request fails
- **THEN** the page shows a concise failure message and a retry control while retaining the publication and review context

### Requirement: Safe dashboard data
The PA#2 demonstration dashboard SHALL use clearly identified synthetic review rows and SHALL NOT request or render protected student identity, source code, model credentials, or review evidence. PA#3 live mode SHALL require assigned-TA access before showing that class's source or evidence; student screens SHALL show only their own judge results. Protected data SHALL NOT enter public shared caches.

#### Scenario: Reviewer inspects page content
- **WHEN** the demonstration dashboard is rendered in any query state
- **THEN** no protected student data, source code, credentials, or cross-class identifiers are present

#### Scenario: PA3-UI-02 Account changes
- **WHEN** a TA logs out and another account signs in on the same browser
- **THEN** the former user's protected source and review data are cleared and unavailable to the new account

### Requirement: Live PA3 authoring submission and review screens

The application SHALL provide signed-in TA lab creation, student code submission and status, and assigned-TA evidence review through the approved API. Controls SHALL work by keyboard and retain reading order on narrow screens. Synthetic data SHALL NOT masquerade as live processing.

#### Scenario: PA3-UI-03 Full screen walkthrough

- **WHEN** the assessor creates a lab, submits the three marking solutions, and opens the TA queue through the application
- **THEN** real persisted results and evidence appear with readable loading and error states and no database-edit or seed workaround

#### Scenario: PA3-UI-04 Narrow evidence view

- **WHEN** the TA reviews code at a narrow viewport
- **THEN** panels stack in reading order, source scrolls within its own region, and every decision remains keyboard accessible
