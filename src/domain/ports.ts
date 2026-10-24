import type { Lab, StoredResponse } from "./types.js";

export interface LabRepository {
  findById(id: string): Promise<Lab | undefined>;
  save(lab: Lab): Promise<void>;
}

export interface IdempotencyRepository {
  find(scope: string): Promise<StoredResponse | undefined>;
  save(scope: string, response: StoredResponse): Promise<void>;
}

export interface Clock {
  now(): Date;
}
