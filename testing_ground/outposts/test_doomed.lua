-- outposts/test_doomed.lua — definition for the doomed remote outpost.
-- Alive during setup, intentionally not started during test runs.
-- The port is the default TEST_DEAD_PORT (19978).
function meta()
    return {
        name        = "Doomed Outpost",
        url         = "http://127.0.0.1:19978/",
        token       = "doomed-token",
        skip        = false,
        notify_down = true,
        sites       = { alternate = "basic" },
    }
end
