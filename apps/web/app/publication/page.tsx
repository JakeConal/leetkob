import type { Metadata } from "next";
import Link from "next/link";

import { Icon } from "../../components/icon";
import { currentLab, isPublicationBlocked, reviewRows } from "../../lib/review-data";

export const metadata: Metadata = {
  title: "Release results | LeeTKOB",
  description: "Understand the deadline, review requirements, and visibility of final lab results.",
};

function ReadinessStep({
  number,
  title,
  detail,
  status,
  state,
}: Readonly<{
  number: string;
  title: string;
  detail: string;
  status: string;
  state: "complete" | "attention" | "locked";
}>) {
  return (
    <li className={`release-step release-step--${state}`}>
      <span className="release-step-number" aria-hidden="true">
        {number}
      </span>
      <div>
        <h3>{title}</h3>
        <p>{detail}</p>
      </div>
      <span className="release-step-status">
        <Icon
          name={state === "complete" ? "check" : state === "attention" ? "clock" : "lock"}
          size={16}
        />
        {status}
      </span>
    </li>
  );
}

function ReleaseReadiness({
  lab,
}: Readonly<{
  lab: { readonly name: string; readonly deadlinePassed: boolean; readonly pendingReviews: number };
}>) {
  const blocked = isPublicationBlocked(lab);
  const reviewsComplete = lab.pendingReviews === 0;
  // This screen explains the gate; PA#3 has no production publication endpoint.
  const releaseAvailable = false;

  return (
    <section className="release-readiness" aria-labelledby="readiness-heading">
      <div className="release-lab-heading">
        <div>
          <p className="release-label">Example lab</p>
          <h2 id="readiness-heading">{lab.name}</h2>
        </div>
        <span className="release-lab-state">
          <span aria-hidden="true" />
          {blocked ? "Not ready to release" : "Checks complete"}
        </span>
      </div>
      <div className="release-summary">
        <strong>
          {lab.pendingReviews}
          <span>pending reviews</span>
        </strong>
        <div>
          <h3>
            {reviewsComplete ? "All example reviews are complete." : "Finish the reviews first."}
          </h3>
          <p>
            {lab.deadlinePassed
              ? "The submission deadline has passed."
              : "The submission deadline has not passed yet."}{" "}
            {reviewsComplete
              ? "No example reviews remain outstanding."
              : "Every required review must be resolved before final marks can be released."}
          </p>
          <a href="#example-reviews">
            View example reviews
            <Icon name="arrow" size={15} />
          </a>
        </div>
      </div>
      <ol className="release-steps" aria-label="Requirements for releasing results">
        <ReadinessStep
          number="01"
          title="Submissions close"
          detail="Wait until the lab’s submission deadline has passed."
          status={lab.deadlinePassed ? "Complete" : "Waiting"}
          state={lab.deadlinePassed ? "complete" : "attention"}
        />
        <ReadinessStep
          number="02"
          title="TA reviews are resolved"
          detail="Resolve every required review in the lab’s private review queue."
          status={reviewsComplete ? "Complete" : `${lab.pendingReviews} remaining`}
          state={reviewsComplete ? "complete" : "attention"}
        />
        <ReadinessStep
          number="03"
          title="Final results are released"
          detail="Release marks and confirmed deductions for the whole lab together."
          status="Unavailable"
          state="locked"
        />
      </ol>
      <div className="release-action">
        <div>
          <strong>Result release is unavailable in this version.</strong>
          <p id="release-unavailable-hint">
            You can review real labs now. Completing reviews does not release marks automatically.
          </p>
        </div>
        <button
          type="button"
          className="secondary-action"
          disabled={blocked || !releaseAvailable}
          aria-describedby="release-unavailable-hint"
        >
          <Icon name="lock" size={16} />
          Release results
        </button>
      </div>
    </section>
  );
}

function ExampleReviews() {
  const needsChanges = reviewRows.filter((row) => row.status === "Changes requested").length;
  const inReview = reviewRows.filter((row) => row.status === "In review").length;
  return (
    <section
      className="release-reviews"
      id="example-reviews"
      aria-labelledby="reviews-heading"
      tabIndex={-1}
    >
      <div className="release-section-heading">
        <h2 id="reviews-heading">Outstanding reviews</h2>
        <span>Example data</span>
      </div>
      <p className="release-section-description">
        {needsChanges} needs changes · {inReview} in review. Open a real lab to view and resolve its
        current findings.
      </p>
      <ul className="release-review-list" aria-label="Example outstanding reviews">
        {reviewRows.map((review) => (
          <li key={review.id}>
            <span className="release-review-number" aria-hidden="true">
              {String(review.id).padStart(2, "0")}
            </span>
            <strong>{review.title}</strong>
            <span
              className={`release-review-status${review.status === "Changes requested" ? " release-review-status--attention" : ""}`}
            >
              {review.status}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function ReleaseGuidance() {
  return (
    <aside className="release-guidance" aria-label="Result release guidance">
      <section className="release-next-step">
        <p className="release-label">Your next step</p>
        <Icon name="review" size={25} />
        <h2>
          Start with
          <br />a real lab.
        </h2>
        <p>
          Choose a lab from your classes, then open its review queue. Review access is limited to
          the assigned TA.
        </p>
        <Link className="primary-action" href="/">
          Open your labs
          <Icon name="arrow" size={17} />
        </Link>
      </section>
      <section className="release-visibility" aria-labelledby="visibility-heading">
        <h2 id="visibility-heading">What can students see?</h2>
        <div>
          <span className="release-visibility-mark" aria-hidden="true">
            <Icon name="check" size={16} />
          </span>
          <h3>Test verdicts</h3>
          <p>Students can see their own submission verdicts and test timings.</p>
        </div>
        <div>
          <span className="release-visibility-mark" aria-hidden="true">
            <Icon name="lock" size={16} />
          </span>
          <h3>Final marks & deductions</h3>
          <p>
            These stay private until results are released. Raw checker findings remain private to
            the TA.
          </p>
        </div>
      </section>
      <details className="release-help">
        <summary>
          Does finishing reviews release marks?
          <Icon name="chevron" size={14} />
        </summary>
        <p>
          No. Resolving reviews and releasing results are separate steps. This version supports
          review decisions; result release is unavailable.
        </p>
      </details>
    </aside>
  );
}

export default function PublicationPage() {
  return (
    <>
      <div className="release-page">
        <header className="release-page-heading">
          <h1>
            Release results<span className="heading-period">.</span>
          </h1>
          <p>
            Understand what needs to happen before final lab marks and reviewed deductions can be
            shared with your class.
          </p>
        </header>
        <div className="release-preview-note">
          <Icon name="labs" size={19} />
          <p>
            <strong>Workflow preview</strong>
            <span>
              This page uses example data to explain result release. It does not show live lab
              progress or publish grades.
            </span>
          </p>
        </div>
        <div className="release-layout">
          <div className="release-main">
            <ReleaseReadiness lab={currentLab} />
            <ExampleReviews />
          </div>
          <ReleaseGuidance />
        </div>
      </div>
    </>
  );
}
