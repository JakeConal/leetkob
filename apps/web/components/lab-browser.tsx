"use client";

import Link from "next/link";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { ApiProblem, getCurrentUser, listLabs } from "../lib/live/api";
import { formatDateTime as formatTimestamp } from "../lib/live/format";
import { filterLabs, labAvailability } from "../lib/lab-catalog";
import { useLiveSession } from "./live/live-shell";
import { Icon } from "./icon";

export function LabBrowser() {
  const auth = useLiveSession();
  const [selectedClass, setSelectedClass] = useState("");
  const [query, setQuery] = useState("");
  const [availability, setAvailability] = useState("All labs");
  const [reload, setReload] = useState(0);
  const [now, setNow] = useState(0);
  const me = useQuery({
    queryKey: ["live", "me"],
    queryFn: getCurrentUser,
    enabled: auth.ready && !auth.closing && auth.csrfToken !== null,
    retry: false,
    gcTime: 0,
    staleTime: 30_000,
  });
  const session =
    me.data === undefined || auth.closing || auth.csrfToken === null
      ? null
      : { user: me.data.user };
  const classes = me.data?.classes ?? [];
  const classId = classes.some((item) => item.id === selectedClass)
    ? selectedClass
    : (classes[0]?.id ?? "");
  const labPages = useInfiniteQuery({
    queryKey: ["live", "catalog", session?.user.id, classId, reload],
    queryFn: ({ pageParam }) => listLabs(classId, pageParam),
    initialPageParam: null as string | null,
    getNextPageParam: (page) => page.nextCursor,
    enabled: session !== null && classId !== "",
    retry: false,
    gcTime: 0,
    staleTime: 0,
  });
  const view = labPages.isError
    ? {
        status: "error" as const,
        problem: {
          title: "Labs unavailable",
          detail:
            labPages.error instanceof ApiProblem
              ? labPages.error.problem.detail
              : "We could not load your labs. Please try again.",
        },
      }
    : labPages.data === undefined && classId !== ""
      ? { status: "loading" as const }
      : {
          status: "ready" as const,
          ownerId: session?.user.id,
          classes,
          classId,
          page: {
            items: labPages.data?.pages.flatMap((page) => page.items) ?? [],
            nextCursor: labPages.data?.pages.at(-1)?.nextCursor ?? null,
          },
        };
  const ready = session !== null && view.status === "ready";
  const labs = ready ? filterLabs(view.page.items, query, availability, now) : [];
  const morePending = labPages.isFetchingNextPage;
  const moreError = labPages.isFetchNextPageError
    ? "Could not load the next page. Try loading more again."
    : null;
  const loadMore = () => labPages.fetchNextPage();
  const tabs = ["All labs", "Open", "Upcoming", "Closed"];
  useEffect(() => {
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);

  return (
    <div className="catalog-layout">
      <section className="catalog" aria-labelledby="catalog-heading">
        <div className="catalog-heading">
          <h2 id="catalog-heading">
            Your labs<span className="heading-period">.</span>
          </h2>
          <span>
            {ready
              ? `${view.page.items.length}${view.page.nextCursor === null ? "" : "+"} labs loaded`
              : "Made for your classroom"}
          </span>
        </div>
        <div className="catalog-tabs" aria-label="Filter labs by availability">
          {tabs.map((tab) => (
            <button
              type="button"
              key={tab}
              aria-pressed={availability === tab}
              className={availability === tab ? "is-selected" : ""}
              onClick={() => setAvailability(tab)}
            >
              {tab}
            </button>
          ))}
        </div>
        <div className="catalog-filters">
          <label className="search-field">
            <Icon name="search" size={18} />
            <input
              aria-label="Search labs"
              type="search"
              placeholder="Search by title or lab ID"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <select
            aria-label="Filter by class"
            value={ready ? view.classId : ""}
            onChange={(e) => setSelectedClass(e.target.value)}
            disabled={!ready || view.classes.length === 0}
          >
            {ready && view.classes.length > 0 ? (
              view.classes.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.name}
                </option>
              ))
            ) : (
              <option value="">Your classes</option>
            )}
          </select>
        </div>
        <div className="table-region catalog-table-region">
          <table className="data-table catalog-table">
            <thead>
              <tr>
                <th scope="col">Lab</th>
                <th scope="col">Availability</th>
                <th scope="col">Deadline</th>
                <th scope="col">Revision</th>
              </tr>
            </thead>
            <tbody>
              {labs.map((lab) => (
                <tr key={lab.id}>
                  <td>
                    <Link href={`/live/labs/${encodeURIComponent(lab.id)}`}>
                      {lab.title}
                      <Icon name="arrow" size={16} />
                    </Link>
                  </td>
                  <td>
                    <span
                      className={`availability availability--${labAvailability(lab, now).toLowerCase()}`}
                    >
                      <i />
                      {labAvailability(lab, now)}
                    </span>
                  </td>
                  <td>{formatTimestamp(lab.deadline)}</td>
                  <td>{String(lab.revision).padStart(2, "0")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {session === null ? (
          <div className="catalog-empty">
            <span className="empty-symbol">
              <Icon name="lock" size={26} />
            </span>
            <h3>Your next challenge is in here.</h3>
            <p>
              Sign in to explore your class labs
              <br />
              and start working on a solution.
            </p>
            <Link href="/live" className="primary-action">
              Sign in to your workspace <Icon name="arrow" size={17} />
            </Link>
            <small>Labs are visible to enrolled class members.</small>
          </div>
        ) : view.status === "error" ? (
          <div className="catalog-empty" role="alert">
            <h3>{view.problem.title}</h3>
            <p>{view.problem.detail}</p>
            <button className="secondary-action" onClick={() => setReload((value) => value + 1)}>
              Retry
            </button>
          </div>
        ) : !ready ? (
          <div className="catalog-empty" role="status">
            <h3>Loading your labs…</h3>
            <p>Getting your classes and available problems.</p>
          </div>
        ) : labs.length === 0 ? (
          <div className="catalog-empty">
            <Icon name="search" size={28} />
            <h3>
              {view.page.items.length === 0 ? "No labs in this class yet." : "No matching labs."}
            </h3>
            <p>
              {view.page.items.length === 0
                ? "Your TA’s labs will appear here when available."
                : "Try a different search or availability filter."}
            </p>
            {query !== "" || availability !== "All labs" ? (
              <button
                className="secondary-action"
                onClick={() => {
                  setQuery("");
                  setAvailability("All labs");
                }}
              >
                Clear filters
              </button>
            ) : null}
          </div>
        ) : null}
        {ready && view.page.nextCursor !== null ? (
          <div className="catalog-pagination">
            <span>Showing {labs.length} matching labs from this page set</span>
            <button
              className="secondary-action"
              disabled={morePending}
              onClick={() => void loadMore()}
            >
              {morePending ? "Loading…" : "Load more labs"}
            </button>
          </div>
        ) : null}
        {session !== null && moreError !== null ? (
          <p className="problem-alert" role="alert">
            {moreError}
          </p>
        ) : null}
        <p className="catalog-footnote">
          <Icon name="clock" size={15} />
          Availability follows the lab schedule. The server confirms submission eligibility.
        </p>
      </section>
      <aside className="catalog-sidebar" aria-label="Practice guide">
        <section className="workspace-summary">
          <span className="summary-index">YOUR WORKSPACE</span>
          <h2>
            {session === null ? (
              <>
                Ready when
                <br />
                you are.
              </>
            ) : (
              <>
                Welcome back,
                <br />
                {session.user.displayName}.
              </>
            )}
          </h2>
          <p>
            {session === null
              ? "One place for your labs, solutions, and feedback."
              : "Find your class, choose a lab, and keep making progress."}
          </p>
          <Link href={session === null ? "/live" : "/live/classes"}>
            {session === null ? "Get started" : "View classes"}
            <Icon name="arrow" size={18} />
          </Link>
        </section>
        <section className="practice-guide">
          <h3>Before you submit</h3>
          <ol>
            <li>
              <span>01</span>
              <div>
                <strong>Check the constraints</strong>
                <p>Review the rules and deductions before you write.</p>
              </div>
            </li>
            <li>
              <span>02</span>
              <div>
                <strong>Try the samples</strong>
                <p>Compare your output with the visible examples.</p>
              </div>
            </li>
            <li>
              <span>03</span>
              <div>
                <strong>Keep an eye on time</strong>
                <p>Submit before the deadline. You can try again while the lab is open.</p>
              </div>
            </li>
          </ol>
        </section>
        <p className="catalog-sidebar-note">
          <Icon name="review" size={18} />
          <span>
            Model-assisted checks.
            <br />
            <strong>Human-reviewed deductions.</strong>
          </span>
        </p>
      </aside>
    </div>
  );
}
