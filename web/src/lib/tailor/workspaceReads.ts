import { api } from '#lib/api.js';
import type { CvAtsDelta, CvFont, CvJobMatch } from '#lib/cv.js';

// The tailoring workspace's accessory reads. None blocks first paint, and none throws.

// The ATS delta for a CV. Never throws: the workspace must load and edit whether or not the
// comparison is available, and a failed read is the same absence as an unavailable one.
export async function readAtsDelta(cvId: string): Promise<CvAtsDelta | null> {
  try {
    return await api.getCvAtsDelta(cvId);
  } catch {
    return null;
  }
}

// The job-match score for a CV. Never throws, for the same reason the delta does not.
export async function readJobMatch(cvId: string): Promise<CvJobMatch | null> {
  try {
    return await api.getCvJobMatch(cvId);
  } catch {
    return null;
  }
}

// The typefaces a CV may pick. Not awaited by the workspace: an empty list only means the font
// picker has nothing to offer yet, and the preview falls back to the template's own face.
export async function readCvFonts(): Promise<CvFont[]> {
  try {
    return await api.listCvFonts();
  } catch {
    return [];
  }
}
