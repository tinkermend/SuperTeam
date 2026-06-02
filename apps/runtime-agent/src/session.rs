#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeSession {
    pub token: String,
    pub expires_at: Option<String>,
}

impl RuntimeSession {
    pub fn new(token: impl Into<String>, expires_at: Option<String>) -> Self {
        Self {
            token: token.into(),
            expires_at,
        }
    }

    pub fn is_empty(&self) -> bool {
        self.token.trim().is_empty()
    }
}
