-- outposts/remote_test.lua — test remote outpost for E2E verification
function meta()
    return {
        name        = "Remote Test Outpost",
        url         = "http://127.0.0.1:8081/check",
        token       = "test-token-123",
        skip        = false,
        notify_down = false,
    }
end
