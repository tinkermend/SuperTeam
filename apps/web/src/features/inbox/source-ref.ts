/**
 * 来源引用的人类可读标签:有名称时显示"名称 (uuid)",否则回退裸 id。
 * 服务端在收件箱读路径批量补名;来源已删除时名称缺省。
 */
export function sourceRefLabel(
  name: string | undefined,
  id: string | undefined,
): string | undefined {
  if (!id) {
    return name || undefined;
  }
  return name ? `${name} (${id})` : id;
}
