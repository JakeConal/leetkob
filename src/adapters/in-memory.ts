import type { IdempotencyRepository, LabRepository } from "../domain/ports.js";
import type { Lab, StoredResponse } from "../domain/types.js";

function cloneLab(lab: Lab): Lab {
  return structuredClone(lab);
}

function cloneResponse(response: StoredResponse): StoredResponse {
  return structuredClone(response);
}

export class InMemoryLabRepository implements LabRepository {
  readonly #labs = new Map<string, Lab>();

  constructor(labs: readonly Lab[] = []) {
    for (const lab of labs) {
      this.#labs.set(lab.id, cloneLab(lab));
    }
  }

  async findById(id: string): Promise<Lab | undefined> {
    const lab = this.#labs.get(id);
    return lab === undefined ? undefined : cloneLab(lab);
  }

  async save(lab: Lab): Promise<void> {
    this.#labs.set(lab.id, cloneLab(lab));
  }
}

export class InMemoryIdempotencyRepository implements IdempotencyRepository {
  readonly #responses = new Map<string, StoredResponse>();

  async find(scope: string): Promise<StoredResponse | undefined> {
    const response = this.#responses.get(scope);
    return response === undefined ? undefined : cloneResponse(response);
  }

  async save(scope: string, response: StoredResponse): Promise<void> {
    if (!this.#responses.has(scope)) {
      this.#responses.set(scope, cloneResponse(response));
    }
  }
}

export class SystemClock {
  now(): Date {
    return new Date();
  }
}
