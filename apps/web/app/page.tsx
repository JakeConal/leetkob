import { Icon } from "../components/icon";
import { LabBrowser } from "../components/lab-browser";

export default function HomePage() {
  return (
    <>
      <section className="catalog-intro" aria-labelledby="page-heading">
        <div>
          <h1 id="page-heading">
            A little practice.
            <br />
            <span>A better programmer.</span>
          </h1>
          <p>
            Your class labs, one focused workspace. Read the problem, write your solution, and put
            it to the test.
          </p>
        </div>
        <div className="intro-index" aria-hidden="true">
          01<span>LABS / PRACTICE</span>
        </div>
      </section>
      <div className="practice-strip" aria-label="How labs work">
        <div>
          <Icon name="labs" />
          <span>
            Read the problem<small>Statement, samples & constraints</small>
          </span>
          <b>01</b>
        </div>
        <div>
          <Icon name="code" />
          <span>
            Write your solution<small>Your approach. C++17.</small>
          </span>
          <b>02</b>
        </div>
        <div>
          <Icon name="check" />
          <span>
            Submit & improve<small>Test verdicts, then TA review</small>
          </span>
          <b>03</b>
        </div>
      </div>
      <LabBrowser />
    </>
  );
}
