/** 与后端 rolevocab roleKeyPattern / 能力词表同规范。 */
const ROLE_KEY_PATTERN = /^[a-z][a-z0-9_]*$/;

export function isValidRoleKey(key: string): boolean {
  return ROLE_KEY_PATTERN.test(key);
}
