// Helpers for building market-facet queries from a search profile's role.

/** Build the facet query for a profile's specialization(s): one `category` param per
 *  specialization. Repeated params are OR-ed by the search backend, so the result is
 *  the combined market across all of the profile's roles. */
export function categoryParams(specializations: string[]): URLSearchParams {
  const params = new URLSearchParams();
  for (const spec of specializations) params.append('category', spec);
  return params;
}
