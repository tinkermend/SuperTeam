//! 平台限额快照(系统配置中心 P2 spec §3)。
//!
//! 控制平面经心跳响应下发生效限额;本模块持有进程内快照,
//! `artifacts.rs`/`skills.rs` 的读取点从这里取值。快照缺失(CP 老版本、
//! 尚未收到首个心跳响应)时回退各限额在源头定义的硬编码默认值——
//! 任何一侧缺失都不破坏现状。执行中任务用启动时取到的值,
//! 收敛粒度=下一次任务,不要求任务中途换限额。

use std::sync::RwLock;

use crate::controlplane::models::PlatformLimits;

/// 生效限额:快照字段缺失处已回填默认值,读取方拿到的总是完整值。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct EffectiveLimits {
    pub version: Option<String>,
    pub artifact_max_file_bytes: usize,
    pub attachment_max_file_bytes: u64,
    pub attachment_max_count: usize,
    pub attachment_total_max_bytes: u64,
    pub skill_archive_max_bytes: u64,
    pub skill_archive_max_file_count: usize,
}

impl Default for EffectiveLimits {
    fn default() -> Self {
        Self {
            version: None,
            artifact_max_file_bytes: crate::artifacts::MAX_ARTIFACT_FILE_BYTES,
            attachment_max_file_bytes: crate::artifacts::MAX_ATTACHMENT_FILE_BYTES,
            attachment_max_count: crate::artifacts::MAX_ATTACHMENT_COUNT,
            attachment_total_max_bytes: crate::artifacts::MAX_ATTACHMENT_TOTAL_BYTES,
            skill_archive_max_bytes: crate::skills::MAX_ARCHIVE_SIZE,
            skill_archive_max_file_count: crate::skills::MAX_FILE_COUNT,
        }
    }
}

static LIMITS: RwLock<Option<EffectiveLimits>> = RwLock::new(None);

/// 当前生效限额快照(无快照时为硬编码默认值)。
pub fn current() -> EffectiveLimits {
    LIMITS
        .read()
        .ok()
        .and_then(|guard| guard.clone())
        .unwrap_or_default()
}

/// 应用心跳携带的快照。指纹(version)没变则不动,避免每 30s 换快照的噪音。
/// 返回 `Some(旧版本描述)` 表示快照发生了变更,调用方据此留一条 info 日志。
pub fn apply(snapshot: &PlatformLimits) -> Option<String> {
    let defaults = EffectiveLimits::default();
    let next = EffectiveLimits {
        version: snapshot.version.clone(),
        artifact_max_file_bytes: snapshot
            .artifact_max_file_size_bytes
            .map(|v| v as usize)
            .unwrap_or(defaults.artifact_max_file_bytes),
        attachment_max_file_bytes: snapshot
            .attachment_max_file_size_bytes
            .unwrap_or(defaults.attachment_max_file_bytes),
        attachment_max_count: snapshot
            .attachment_max_count
            .map(|v| v as usize)
            .unwrap_or(defaults.attachment_max_count),
        attachment_total_max_bytes: snapshot
            .attachment_total_max_bytes
            .unwrap_or(defaults.attachment_total_max_bytes),
        skill_archive_max_bytes: snapshot
            .skill_archive_max_bytes
            .unwrap_or(defaults.skill_archive_max_bytes),
        skill_archive_max_file_count: snapshot
            .skill_archive_max_file_count
            .map(|v| v as usize)
            .unwrap_or(defaults.skill_archive_max_file_count),
    };
    let mut guard = match LIMITS.write() {
        Ok(guard) => guard,
        Err(_) => return None,
    };
    let previous_version = match guard.as_ref() {
        Some(existing) if *existing == next => return None,
        Some(existing) => existing
            .version
            .clone()
            .unwrap_or_else(|| "unversioned".to_string()),
        None => "local-defaults".to_string(),
    };
    *guard = Some(next);
    Some(previous_version)
}

#[cfg(test)]
pub fn reset_for_test() {
    if let Ok(mut guard) = LIMITS.write() {
        *guard = None;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // 快照测试串行跑(进程级全局);cargo test 默认并行,用同一把锁保护。
    static TEST_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());

    #[test]
    fn missing_snapshot_falls_back_to_defaults() {
        let _lock = TEST_LOCK.lock().unwrap();
        reset_for_test();
        let limits = current();
        assert_eq!(
            limits.artifact_max_file_bytes,
            crate::artifacts::MAX_ARTIFACT_FILE_BYTES
        );
        assert_eq!(limits.skill_archive_max_bytes, crate::skills::MAX_ARCHIVE_SIZE);
        assert!(limits.version.is_none());
    }

    #[test]
    fn apply_merges_partial_snapshot_over_defaults() {
        let _lock = TEST_LOCK.lock().unwrap();
        reset_for_test();
        let snapshot = PlatformLimits {
            version: Some("plv1:sha256:abc".to_string()),
            artifact_max_file_size_bytes: Some(20 * 1024 * 1024),
            ..Default::default()
        };
        let changed = apply(&snapshot);
        assert_eq!(changed, Some("local-defaults".to_string()));
        let limits = current();
        assert_eq!(limits.artifact_max_file_bytes, 20 * 1024 * 1024);
        // 未下发的字段维持默认值。
        assert_eq!(
            limits.attachment_max_file_bytes,
            crate::artifacts::MAX_ATTACHMENT_FILE_BYTES
        );
        reset_for_test();
    }

    #[test]
    fn apply_is_noop_when_unchanged() {
        let _lock = TEST_LOCK.lock().unwrap();
        reset_for_test();
        let snapshot = PlatformLimits {
            version: Some("plv1:sha256:same".to_string()),
            attachment_max_count: Some(5),
            ..Default::default()
        };
        assert!(apply(&snapshot).is_some());
        assert!(apply(&snapshot).is_none());
        let updated = PlatformLimits {
            version: Some("plv1:sha256:next".to_string()),
            attachment_max_count: Some(7),
            ..Default::default()
        };
        assert_eq!(apply(&updated), Some("plv1:sha256:same".to_string()));
        assert_eq!(current().attachment_max_count, 7);
        reset_for_test();
    }
}
