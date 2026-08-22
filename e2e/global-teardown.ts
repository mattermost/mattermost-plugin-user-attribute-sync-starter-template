import * as fs from 'fs/promises';

import {adminStorageStatePath} from './constants';

/** Runs once after every test. Removes the saved session rather than leaving it on disk. */
async function globalTeardown() {
    await fs.rm(adminStorageStatePath, {force: true});
}

export default globalTeardown;
