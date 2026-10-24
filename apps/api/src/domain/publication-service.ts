import type { Clock, IdempotencyRepository, LabRepository } from "./ports.js";
import { problemResponse } from "./problems.js";
import type {
  Lab,
  PublicationRepresentation,
  PublishInput,
  PublishedLab,
  StoredResponse,
} from "./types.js";

export class PublicationService {
  readonly #tails = new Map<string, Promise<void>>();

  constructor(
    private readonly labs: LabRepository,
    private readonly idempotency: IdempotencyRepository,
    private readonly clock: Clock,
  ) {}

  async publish(input: PublishInput): Promise<StoredResponse> {
    return this.#exclusive(input.labId, async () => {
      const scope = JSON.stringify([input.identity.subject, input.labId, input.idempotencyKey]);
      const replay = await this.idempotency.find(scope);
      if (replay !== undefined) return replay;

      const response = await this.#evaluate(input);
      await this.idempotency.save(scope, response);
      return response;
    });
  }

  async #evaluate(input: PublishInput): Promise<StoredResponse> {
    if (input.identity.role !== "ta") {
      return problemResponse(
        "ta-role-required",
        403,
        "TA role required",
        "Only a teaching assistant may publish a lab.",
        input.instance,
      );
    }

    const lab = await this.labs.findById(input.labId);
    if (lab === undefined || !input.identity.classIds.includes(lab.classId)) {
      return problemResponse(
        "lab-not-found",
        404,
        "Lab not found",
        "No lab is available to this teaching assistant at the requested identifier.",
        input.instance,
      );
    }

    if (lab.status === "published") {
      return this.#alreadyPublished(lab, input.instance);
    }

    if (lab.pendingReviews > 2) {
      return problemResponse(
        "reviews-pending",
        409,
        "Reviews pending",
        "All required reviews must be resolved before marks can be published.",
        input.instance,
        { pendingReviews: lab.pendingReviews },
      );
    }

    const publishedAt = this.clock.now().toISOString();
    const published: PublishedLab = {
      id: lab.id,
      classId: lab.classId,
      status: "published",
      pendingReviews: 0,
      publishedAt,
      publishedBy: input.identity.subject,
      publicationKey: input.idempotencyKey,
    };
    await this.labs.save(published);

    const body: PublicationRepresentation = {
      labId: published.id,
      status: "published",
      publishedAt,
      publishedBy: input.identity.subject,
    };
    return { status: 200, contentType: "application/json", body };
  }

  #alreadyPublished(lab: PublishedLab, instance: string): StoredResponse {
    return problemResponse(
      "lab-already-published",
      409,
      "Lab already published",
      "The lab was already published using a different idempotency key.",
      instance,
      {
        labId: lab.id,
        publishedAt: lab.publishedAt,
        publishedBy: lab.publishedBy,
      },
    );
  }

  async #exclusive<T>(key: string, work: () => Promise<T>): Promise<T> {
    const previous = this.#tails.get(key) ?? Promise.resolve();
    let release = (): void => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const tail = previous.then(() => gate);
    this.#tails.set(key, tail);

    await previous;
    try {
      return await work();
    } finally {
      release();
      if (this.#tails.get(key) === tail) this.#tails.delete(key);
    }
  }
}

export function isPublished(lab: Lab | undefined): lab is PublishedLab {
  return lab?.status === "published";
}
