/** Constants that are about the app rather than about any one trip. Trip data
 *  itself now lives on the server — see server/seed for where these two plans
 *  started life. */

/** Weather marks, in the order the buttons appear. */
export const WX = ["☀", "⛅", "☂", "≡"] as const;
export const WX_NAME = ["Sun", "Cloud", "Rain", "Fog"] as const;
