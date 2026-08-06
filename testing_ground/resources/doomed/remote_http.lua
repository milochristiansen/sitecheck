function meta()
    return {
        name        = "Remote HTTP Check",
        description = "HTTP check from remote outpost against test server",
        sites       = { alternate = "basic" },
    }
end

function check()
    local r = http_fetch("http://127.0.0.1:19976/ok", {
        method = "GET",
        timeout = 5,
    })

    if r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    if r.StatusCode == 200 then
        r.Pass = PASS
        return r
    end

    r.Pass = FAIL
    r.FailReason = "expected 200, got " .. r.StatusCode
    return r
end
