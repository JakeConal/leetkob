import { SystemStatus } from "../components/system-status";
import { currentLab, isPublicationBlocked, reviewRows } from "../lib/review-data";

export default function HomePage() {
  const publicationBlocked = isPublicationBlocked(currentLab);

  return (
    <div className="live-page">
      <header className="live-page-head">
        <div className="live-heading-group">
          <h1 id="page-heading">Publication control</h1>
          <p className="live-muted">
            Review the current lab status, resolve pending reviews, and publish when all checks
            pass.
          </p>
        </div>
        <span className="live-status">Development / PA#2</span>
      </header>

      <section className="live-gate" aria-labelledby="gate-heading">
        <div>
          <h2 id="gate-heading">Publication blocked</h2>
          <p>The deadline has passed. Resolve every review before publishing.</p>
          <dl className="live-gate-facts live-gate-facts--dl">
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
        </div>
        <button type="button" disabled={publicationBlocked}>
          Publish lab
        </button>
      </section>

      <div className="live-home-grid">
        <section className="live-section" aria-labelledby="review-heading">
          <div className="live-card-head">
            <h2 id="review-heading">Review queue</h2>
            <span className="live-status live-status--warn">Demonstration data</span>
          </div>
          <div className="live-panel live-table-wrap">
            <table className="live-table live-table--stack" aria-label="Pending review queue">
              <thead>
                <tr>
                  <th scope="col" className="live-num">
                    No.
                  </th>
                  <th scope="col">Constraint</th>
                  <th scope="col">Status</th>
                  <th scope="col">Updated</th>
                </tr>
              </thead>
              <tbody>
                {reviewRows.map((review) => (
                  <tr key={review.id}>
                    <td data-label="No." className="live-num live-muted">
                      {String(review.id).padStart(2, "0")}
                    </td>
                    <th scope="row" data-label="Constraint">
                      {review.title}
                    </th>
                    <td data-label="Status">
                      <span
                        className={`live-status ${review.status === "Changes requested" ? "live-status--bad" : "live-status--warn"}`}
                      >
                        {review.status}
                      </span>
                    </td>
                    <td data-label="Updated">
                      <time>{review.updated}</time>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>

        <SystemStatus />
      </div>
    </div>
  );
}
