function meta()
    return {
        name        = "TCP Fail",
        description = "Checks TCP connectivity to a closed port — always fails",
        sites       = { alternate = "basic" },
    }
end

function check()
    local r = tcp_connect("127.0.0.1", 19978, {
        timeout = 3,
    })

    if r.Error ~= nil and r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    r.Pass = FAIL
    r.FailReason = "expected connection refused, but connected successfully"
    return r
end
