function meta()
    return {
        name        = "HTTP Fail",
        description = "Checks a non-existent server — always fails",
        sites       = { alternate = "basic" },
    }
end

function check()
    local r = http_fetch("http://127.0.0.1:19978/ok", {
        method = "GET",
        timeout = 3,
    })

    -- No server on 19978 — the error field will be populated.
    if r.Error ~= nil and r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    -- If by some miracle we connected, still mark fail — this isn't a real server.
    r.Pass = FAIL
    r.FailReason = "unexpected success connecting to dead port"
    return r
end
