-- outposts/test_remote.lua — definition for a second local scoutpost instance
-- Used in multi-outpost tests. The port is the default TEST_REMOTE_PORT (19977).
function meta()
    return {
        name        = "Test Remote Outpost",
        url         = "http://127.0.0.1:19977/",
        token       = "test-remote-token",
        skip        = false,
        notify_down = true,
    }
end
