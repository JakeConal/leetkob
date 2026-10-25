import { SystemStatus } from "../components/system-status";
import { currentLab, isPublicationBlocked, reviewRows } from "../lib/review-data";

export default function HomePage() {
  const publicationBlocked = isPublicationBlocked(currentLab);

  return (
    <main className="page-shell">
      <header className="masthead">
        <a className="wordmark" href="#main-content" aria-label="LeeTKOB home">
          LeeTKOB
        </a>
        <p className="environment">
          <span aria-hidden="true" /> Development / PA#2
        </p>
      </header>

      <div className="workspace" id="main-content">
        <div className="primary-column">
          <section className="intro" aria-labelledby="page-heading">
            <div className="section-index" aria-hidden="true">
              01
            </div>
            <div>
              <h1 id="page-heading">Publication control</h1>
              <p>
                Review the current lab status, resolve pending reviews, and publish when all checks
                pass.
              </p>
            </div>
          </section>

          <section className="publication-gate" aria-labelledby="gate-heading">
            <div className="gate-signal" aria-hidden="true">
              !
            </div>
            <div className="gate-copy">
              <p className="gate-state">Blocked</p>
              <h2 id="gate-heading">Publication blocked</h2>
              <p>The deadline has passed. Resolve every review before publishing.</p>
            </div>
            <dl className="gate-facts">
              <div>
                <dt>API status</dt>
                <dd>Live check</dd>
              </div>
              <div>
                <dt>Current lab</dt>
                <dd>{currentLab.name}</dd>
              </div>
              <div>
                <dt>Submission deadline</dt>
                <dd>Passed</dd>
              </div>
              <div>
                <dt>Pending reviews</dt>
                <dd>{currentLab.pendingReviews}</dd>
              </div>
            </dl>
            <button className="publish-button" type="button" disabled={publicationBlocked}>
              Publish lab
            </button>
          </section>

          <section className="review-section" aria-labelledby="review-heading">
            <div className="review-heading-row">
              <h2 id="review-heading">Review queue</h2>
              <p>Demonstration data</p>
            </div>
            <div className="review-table" role="table" aria-label="Pending review queue">
              <div className="review-row review-row--header" role="row">
                <span role="columnheader">No.</span>
                <span role="columnheader">Constraint</span>
                <span role="columnheader">Status</span>
                <span role="columnheader">Updated</span>
              </div>
              {reviewRows.map((review) => (
                <div className="review-row" role="row" key={review.id}>
                  <span role="cell" data-label="No.">
                    {String(review.id).padStart(2, "0")}
                  </span>
                  <strong role="cell" data-label="Constraint">
                    {review.title}
                  </strong>
                  <span role="cell" data-label="Status" className="review-status">
                    <i aria-hidden="true" /> {review.status}
                  </span>
                  <time role="cell" data-label="Updated">
                    {review.updated}
                  </time>
                </div>
              ))}
            </div>
          </section>
        </div>

        <SystemStatus />
      </div>

      <footer className="footer">
        <p>LeeTKOB reviewer workspace</p>
        <p>PA#2 / Local development</p>
      </footer>
    </main>
  );
}
